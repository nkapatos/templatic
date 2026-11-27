package generate

import (
	"fmt"
	"go/types"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

type propsTypes struct {
	Interfaces map[string]string
	Fields     map[string][]string
	Warnings   []string
	Names      []string
}

// convertPropsTypes renders the struct type typeName, declared in the package at fileDir, and every struct it reaches as TS interfaces.
func convertPropsTypes(prog *program, fileDir, typeName string) (*propsTypes, error) {
	pkg := prog.pkgInDir(filepath.Join(prog.root, filepath.FromSlash(fileDir)))
	var st *types.Struct
	var root *types.Named
	if pkg != nil && pkg.Types != nil {
		if tn, ok := pkg.Types.Scope().Lookup(typeName).(*types.TypeName); ok {
			t := types.Unalias(tn.Type())
			root, _ = t.(*types.Named)
			st, _ = t.Underlying().(*types.Struct)
		}
	}
	if st == nil {
		return nil, fmt.Errorf("generate: props type %s not found in %s (exported struct required)", typeName, fileDir)
	}
	if err := pkgErr(pkg); err != nil {
		return nil, err
	}
	return convertStruct(root, st, typeName)
}

// convertStruct renders st, named typeName in TS and declared as root (nil when anonymous), with the structs it reaches; a type json/v2 cannot marshal is an error.
func convertStruct(root *types.Named, st *types.Struct, typeName string) (*propsTypes, error) {
	c := &tsConv{out: &propsTypes{Interfaces: map[string]string{}, Fields: map[string][]string{}}, names: map[string]string{}, used: map[string]bool{typeName: true}}
	if root != nil {
		c.names[root.String()] = typeName
	}
	c.out.Interfaces[typeName] = c.interfaceBody(typeName, st)

	refs := make([]string, 0, len(c.out.Interfaces))
	for name := range c.out.Interfaces {
		if name != typeName {
			refs = append(refs, name)
		}
	}
	slices.Sort(refs)
	c.out.Names = append([]string{typeName}, refs...)
	slices.Sort(c.out.Warnings)
	if len(c.errs) > 0 {
		return nil, fmt.Errorf("generate: type %s: %s", typeName, strings.Join(c.errs, "; "))
	}
	return c.out, nil
}

type tsConv struct {
	out   *propsTypes
	names map[string]string
	used  map[string]bool
	field string // the field being converted, for errors
	errs  []string
}

type jsonField struct {
	name     string
	tagged   bool
	depth    int
	index    []int
	typ      types.Type
	optional bool
	quoted   bool
}

// jsonFields lists the fields encoding/json/v2 marshals for st, in its depth-first order.
func jsonFields(st *types.Struct) ([]jsonField, []string) {
	all, bad := collectFields(st)
	byName := map[string][]jsonField{}
	for _, f := range all {
		byName[f.name] = append(byName[f.name], f)
	}
	var win []jsonField
	for _, group := range byName {
		if f, ok := dominantField(group); ok {
			win = append(win, f)
		}
	}
	slices.SortFunc(win, func(a, b jsonField) int { return slices.Compare(a.index, b.index) })
	return win, bad
}

// dominantField applies v2's rule: the shallowest field wins, a lone tagged one breaks a tie, else the name is dropped.
func dominantField(fs []jsonField) (jsonField, bool) {
	shallow := slices.MinFunc(fs, func(a, b jsonField) int { return a.depth - b.depth }).depth
	var top, tagged []jsonField
	for _, f := range fs {
		if f.depth == shallow {
			top = append(top, f)
			if f.tagged {
				tagged = append(tagged, f)
			}
		}
	}
	switch {
	case len(top) == 1:
		return top[0], true
	case len(tagged) == 1:
		return tagged[0], true
	}
	return jsonField{}, false
}

// collectFields walks st and its embedded structs breadth-first as v2 does: a struct type reached again is read, but its own embedded structs are not followed twice.
// collectFields walks st as json/v2 does; bad lists the fields it refuses to marshal.
func collectFields(st *types.Struct) (all []jsonField, bad []string) {
	type entry struct {
		st    *types.Struct
		index []int
		visit bool
	}
	queue := []entry{{st, nil, true}}
	seen := map[*types.Struct]bool{st: true}
	for i := 0; i < len(queue); i++ {
		e := queue[i]
		for j := range e.st.NumFields() {
			v := e.st.Field(j)
			tag := parseJSONTag(e.st.Tag(j))
			if tag.omit {
				continue
			}
			if v.Embedded() && tag.name == "" || tag.embed {
				t := v.Type()
				if p, ok := types.Unalias(t).(*types.Pointer); ok {
					t = p.Elem()
				}
				if inner, ok := types.Unalias(t).Underlying().(*types.Struct); ok {
					if e.visit {
						queue = append(queue, entry{inner, append(slices.Clone(e.index), j), !seen[inner]})
					}
					seen[inner] = true
					continue
				}
				if tag.name == "" {
					bad = append(bad, fmt.Sprintf("embedded field %s of non-struct type needs a JSON name", v.Name()))
					continue
				}
			}
			if !v.Exported() {
				continue
			}
			n := v.Name()
			if tag.name != "" {
				n = tag.name
			}
			all = append(all, jsonField{name: n, tagged: tag.name != "", depth: len(e.index), index: append(slices.Clone(e.index), j), typ: v.Type(), optional: tag.omitzero || tag.omitempty && canEncodeEmpty(v.Type()), quoted: tag.quoted})
		}
	}
	return all, bad
}

type jsonTag struct {
	name                string
	omit, embed         bool
	omitempty, omitzero bool
	quoted              bool
}

// parseJSONTag reads the json key the way v2 does: a name without quotes or backslashes, then comma-separated options.
func parseJSONTag(raw string) jsonTag {
	tag, ok := reflect.StructTag(raw).Lookup("json")
	if !ok {
		return jsonTag{}
	}
	if tag == "-" {
		return jsonTag{omit: true}
	}
	name, opts, _ := strings.Cut(tag, ",")
	var t jsonTag
	if validJSONName(name) {
		t.name = name
	}
	for opt := range strings.SplitSeq(opts, ",") {
		opt, _, _ = strings.Cut(opt, ":")
		switch opt {
		case "omitempty":
			t.omitempty = true
		case "omitzero":
			t.omitzero = true
		case "string":
			t.quoted = true
		case "embed":
			t.embed = true
		}
	}
	return t
}

func validJSONName(s string) bool {
	return s != "" && utf8.ValidString(s) && !strings.ContainsAny(s, "\\'\"`")
}

var tsIdent = regexp.MustCompile(`^[A-Za-z_$][A-Za-z0-9_$]*$`)

func tsPropName(name string) string {
	if tsIdent.MatchString(name) {
		return name
	}
	return tsString(name)
}

func (c *tsConv) fieldsBody(st *types.Struct, indent string, names *[]string) string {
	var sb strings.Builder
	fields, bad := jsonFields(st)
	c.errs = append(c.errs, bad...)
	for _, f := range fields {
		c.field = f.name
		tsType := c.fieldType(f)
		if names != nil {
			*names = append(*names, f.name)
		}
		opt := ""
		if f.optional {
			opt = "?"
			tsType = strings.TrimSuffix(tsType, " | null")
		}
		sb.WriteString(indent + tsPropName(f.name) + opt + ": " + tsType + ";\n")
	}
	return sb.String()
}

// fieldType is the TS type of f; a number under the `string` tag option is quoted, through pointers.
func (c *tsConv) fieldType(f jsonField) string {
	if f.quoted {
		t, ptr := f.typ, false
		for {
			p, ok := types.Unalias(t).(*types.Pointer)
			if !ok {
				break
			}
			t, ptr = p.Elem(), true
		}
		if b, ok := types.Unalias(t).Underlying().(*types.Basic); ok && b.Info()&(types.IsInteger|types.IsFloat) != 0 && !hasMarshaler(t) {
			if ptr {
				return "string | null"
			}
			return "string"
		}
	}
	return c.tsType(f.typ)
}

func (c *tsConv) interfaceBody(typeName string, st *types.Struct) string {
	var names []string
	body := c.fieldsBody(st, "  ", &names)
	c.out.Fields[typeName] = names
	return body
}

// interfaceName is the TS name of a named struct: its Go name, qualified by package then numbered when another type already took it.
func (c *tsConv) interfaceName(t *types.Named) string {
	key := t.String()
	if n, ok := c.names[key]; ok {
		return n
	}
	obj := t.Obj()
	base := obj.Name()
	for i := range t.TypeArgs().Len() {
		base += "_" + tsIdentSafe(types.TypeString(t.TypeArgs().At(i), func(p *types.Package) string { return p.Name() }))
	}
	name := base
	if c.used[name] && obj.Pkg() != nil {
		name = pascal(obj.Pkg().Name()) + base
	}
	for i := 2; c.used[name]; i++ {
		name = fmt.Sprintf("%s%d", base, i)
	}
	c.used[name] = true
	c.names[key] = name
	return name
}

func tsIdentSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '_' || r == '$' || unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return '_'
	}, s)
}

func mapKeyTS(key types.Type) string {
	if b, ok := key.Underlying().(*types.Basic); ok && b.Info()&types.IsInteger != 0 {
		return "number"
	}
	return "string"
}

func (c *tsConv) tsType(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Basic:
		switch info := t.Info(); {
		case info&types.IsString != 0:
			return "string"
		case info&types.IsBoolean != 0:
			return "boolean"
		case info&(types.IsInteger|types.IsFloat) != 0:
			return "number"
		}
	case *types.Named:
		obj := t.Obj()
		if obj.Pkg() != nil {
			switch obj.Pkg().Path() + "." + obj.Name() {
			case "time.Time":
				return "string"
			case "time.Duration":
				c.errs = append(c.errs, fmt.Sprintf("field %s: time.Duration has no json/v2 encoding (use an int64 or a string)", c.field))
				return "unknown"
			// live.Strings is always an array in props: the server marshals the slice.
			case liveImport + ".Strings":
				return "string[]"
			// The runtime unwraps a bound collection's snapshot into its items.
			case liveImport + ".Snapshot":
				return c.sliceTS(t.TypeArgs().At(0))
			}
		}
		// v2 prefers a type's own marshaler to its structure.
		if hasJSONMarshaler(t) {
			break
		}
		if hasTextMarshaler(t) {
			return "string"
		}
		st, ok := t.Underlying().(*types.Struct)
		if !ok {
			return c.tsType(t.Underlying())
		}
		_, done := c.names[t.String()]
		name := c.interfaceName(t)
		if done {
			return name
		}
		// Reserve the name before recursing so a self-referential struct does not loop.
		c.out.Interfaces[name] = ""
		c.out.Interfaces[name] = c.interfaceBody(name, st)
		return name
	case *types.Pointer:
		inner := c.tsType(t.Elem())
		if inner == "unknown" || strings.HasSuffix(inner, " | null") {
			return inner
		}
		return inner + " | null"
	case *types.Slice:
		if isByte(t.Elem()) {
			return "string" // base64
		}
		return c.sliceTS(t.Elem())
	case *types.Array:
		if isByte(t.Elem()) {
			return "string" // base64
		}
		return c.sliceTS(t.Elem())
	case *types.Map:
		value := c.tsType(t.Elem())
		return "Record<" + mapKeyTS(t.Key()) + ", " + value + ">"
	case *types.Interface:
		return "unknown"
	case *types.Struct:
		body := c.fieldsBody(t, "    ", nil)
		return "{\n" + body + "  }"
	}
	c.out.Warnings = append(c.out.Warnings, fmt.Sprintf("%s → unknown", types.TypeString(t, func(p *types.Package) string { return p.Name() })))
	return "unknown"
}

// canEncodeEmpty reports whether a value of t can encode as null, "", {} or [], the values v2's omitempty omits.
func canEncodeEmpty(t types.Type) bool {
	if hasMethod(t, "MarshalJSON") || hasMethod(t, "MarshalJSONTo") || hasMethod(t, "MarshalText") || hasMethod(t, "AppendText") {
		return true
	}
	switch u := t.Underlying().(type) {
	case *types.Basic:
		return u.Info()&types.IsString != 0
	case *types.Array:
		return u.Len() == 0
	}
	return true
}

// hasMethod reports whether t or *t has the named method, as v2 looks up marshalers.
func hasMethod(t types.Type, name string) bool {
	return types.NewMethodSet(types.NewPointer(t)).Lookup(nil, name) != nil
}

// isByte reports whether a slice or array of t is v2's base64 case: a byte element without a marshaler of its own.
func isByte(t types.Type) bool {
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Kind() == types.Uint8 && !hasMarshaler(t)
}

func hasJSONMarshaler(t types.Type) bool {
	return hasMethod(t, "MarshalJSON") || hasMethod(t, "MarshalJSONTo")
}

func hasTextMarshaler(t types.Type) bool {
	return hasMethod(t, "MarshalText") || hasMethod(t, "AppendText")
}

func hasMarshaler(t types.Type) bool {
	return hasJSONMarshaler(t) || hasTextMarshaler(t)
}

func (c *tsConv) sliceTS(elem types.Type) string {
	t := c.tsType(elem)
	if strings.HasSuffix(t, " | null") {
		t = "(" + t + ")"
	}
	return t + "[]"
}
