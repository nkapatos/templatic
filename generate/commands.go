package generate

import (
	"cmp"
	"fmt"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const commandsHeader = "// @tc-generated: overwritten by templatic generate. Edit the Go live.NewCommand declarations.\n"

type commandDecl struct {
	name string
	ts   string
}

func writeCommandsTS(opts Options, prog *program) error {
	path := filepath.Join(opts.genDir(), "commands.ts")

	nsOf := map[string]string{}
	taken := map[string]bool{}
	allocNS := func(pkg *types.Package) string {
		if ns, ok := nsOf[pkg.Path()]; ok {
			return ns
		}
		ns := pkg.Name()
		for i := 2; taken[ns]; i++ {
			ns = fmt.Sprintf("%s%d", pkg.Name(), i)
		}
		taken[ns] = true
		nsOf[pkg.Path()] = ns
		return ns
	}
	for _, pkg := range prog.pkgs {
		if pkg.Types != nil {
			allocNS(pkg.Types)
		}
	}

	var cmds []commandDecl
	var skipped, notes []string
	nsBodies := map[string]map[string]string{}
	seen := map[string]bool{}
	for _, c := range prog.calls {
		if c.fn != "NewCommand" || len(c.call.Args) != 1 || c.targs.Len() != 1 {
			continue
		}
		cmd, ok := c.constString(0)
		if !ok {
			skipped = append(skipped, c.where+" (computed name)")
			continue
		}
		if seen[cmd] {
			continue
		}
		seen[cmd] = true
		in := types.Unalias(c.targs.At(0))
		ts := "unknown"
		named, _ := in.(*types.Named)
		switch st, _ := in.Underlying().(*types.Struct); {
		case named == nil && st != nil && st.NumFields() == 0:
			ts = "Record<string, never>"
		case named != nil && st != nil && named.Obj().Pkg() != nil:
			typ := named.Obj().Name()
			if err := prog.pkgErr(named.Obj().Pkg().Path()); err != nil {
				notes = append(notes, fmt.Sprintf("%s: %s: %v", cmd, c.where, err))
				break
			}
			conv, err := convertStruct(named, st, typ)
			if err != nil {
				return fmt.Errorf("%s: %s: %w", cmd, c.where, err)
			}
			ns := allocNS(named.Obj().Pkg())
			if nsBodies[ns] == nil {
				nsBodies[ns] = map[string]string{}
			}
			for _, n := range conv.Names {
				nsBodies[ns][n] = conv.Interfaces[n]
			}
			for _, w := range conv.Warnings {
				notes = append(notes, ns+"."+w)
			}
			ts = ns + "." + typ
		default:
			notes = append(notes, fmt.Sprintf("%s: %s: input type %s is not a named struct → unknown", cmd, c.where, in))
		}
		cmds = append(cmds, commandDecl{name: cmd, ts: ts})
	}

	if len(cmds) == 0 {
		if old, err := os.ReadFile(path); err == nil && strings.HasPrefix(string(old), commandsHeader) {
			return os.Remove(path)
		}
		return nil
	}
	slices.SortFunc(cmds, func(a, b commandDecl) int { return cmp.Compare(a.name, b.name) })

	var b strings.Builder
	b.WriteString(commandsHeader)
	b.WriteString("// Typed commands: every live.NewCommand[In](\"name\") under the app root.\n")
	b.WriteString("// The runtime's untyped send stays available from \"templatic\".\n\n")
	b.WriteString("import { send as rawSend } from \"templatic\";\n\n")
	for _, ns := range slices.Sorted(maps.Keys(nsBodies)) {
		fmt.Fprintf(&b, "export declare namespace %s {\n", ns)
		for _, n := range slices.Sorted(maps.Keys(nsBodies[ns])) {
			var fields []string
			for _, l := range strings.Split(strings.TrimRight(nsBodies[ns][n], "\n"), "\n") {
				fields = append(fields, "\t\t"+strings.TrimSpace(l))
			}
			fmt.Fprintf(&b, "\tinterface %s {\n%s\n\t}\n", n, strings.Join(fields, "\n"))
		}
		b.WriteString("}\n\n")
	}
	b.WriteString("export interface Commands {\n")
	for _, c := range cmds {
		fmt.Fprintf(&b, "\t%s: %s;\n", tsString(c.name), c.ts)
	}
	b.WriteString("}\n\nexport type CommandName = keyof Commands;\n\n")
	b.WriteString("// send raises a declared command; R is the handler's result, untyped on the Go side.\n")
	b.WriteString("export function send<K extends CommandName, R = unknown>(name: K, args: Commands[K]): Promise<R | null> {\n\treturn rawSend<R>(name, args);\n}\n")
	slices.Sort(notes)
	for _, n := range append(skipped, notes...) {
		b.WriteString("// note: " + n + "\n")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return writeGenerated(path, b.String())
}
