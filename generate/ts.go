package generate

import (
	"encoding/json/jsontext"
	"encoding/json/v2"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func writeIslandTS(opts Options, prog *program, isl Island) error {
	gen := filepath.Join(opts.genDir(), "islands")
	if err := os.MkdirAll(gen, 0o755); err != nil {
		return err
	}
	types, err := convertPropsTypes(prog, isl.FileDir, isl.PropsType)
	if err != nil {
		return err
	}
	for _, b := range isl.Binds {
		found := false
		for _, name := range types.Fields[isl.PropsType] {
			if name == b.Field {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("generate: %s: island %s: bind=%s:%s names no field of props struct %s (json field names: %s)",
				isl.File, isl.Tag, b.Field, b.Topic, isl.PropsType, strings.Join(types.Fields[isl.PropsType], ", "))
		}
	}
	dir := filepath.Join(opts.Root, opts.ClientDir, "islands")
	base := pascal(isl.Tag)
	isl.Script = rebaseRelative(opts, isl.Script, jsRelImport)
	isl.CSS = rebaseRelative(opts, isl.CSS, cssRelImport)

	if err := writeGenerated(filepath.Join(gen, isl.Tag+".types.ts"), propsTypesSource(isl, types)); err != nil {
		return err
	}
	if err := writeGenerated(filepath.Join(gen, isl.Tag+".hooks.ts"), hooksSource(isl)); err != nil {
		return err
	}
	if err := writeGenerated(filepath.Join(gen, isl.Tag+".base.ts"), baseSource(isl, base)); err != nil {
		return err
	}

	cssPath := filepath.Join(gen, isl.Tag+".css")
	if strings.TrimSpace(isl.CSS) != "" {
		if err := writeGenerated(cssPath, isl.CSS); err != nil {
			return err
		}
	} else if _, err := os.Stat(cssPath); err == nil {
		if err := os.Remove(cssPath); err != nil {
			return err
		}
	}

	definePath := filepath.Join(dir, isl.Tag+".ts")
	if _, err := os.Stat(definePath); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := writeGenerated(definePath, defineSource(isl, base)); err != nil {
			return err
		}
	}
	return nil
}

const generatedHeader = "// @tc-generated: overwritten by templatic generate. Edit the source .templ file.\n"

func propsTypesSource(isl Island, types *propsTypes) string {
	var sb strings.Builder
	sb.WriteString(generatedHeader)
	sb.WriteString("\n")
	for _, name := range types.Names {
		fmt.Fprintf(&sb, "export interface %s {\n%s}\n\n", name, types.Interfaces[name])
	}
	for _, w := range types.Warnings {
		sb.WriteString("// note: ")
		sb.WriteString(w)
		sb.WriteString("\n")
	}
	return sb.String()
}

func hooksSource(isl Island) string {
	host := "HTMLElement"
	litImport := ""
	if isl.Variant == "lit" {
		host = "LitElement"
		litImport = "import type { LitElement } from \"lit\";\n"
	}
	props := isl.PropsType

	var sb strings.Builder
	sb.WriteString(generatedHeader)
	sb.WriteString("\n")
	sb.WriteString(litImport)
	fmt.Fprintf(&sb, "import type { %s } from \"./%s.types\";\n", props, isl.Tag)
	sb.WriteString("import type { IslandContext } from \"../runtime/context\";\n")
	if strings.TrimSpace(isl.CSS) != "" {
		fmt.Fprintf(&sb, "import \"./%s.css\";\n", isl.Tag)
	}
	sb.WriteString("\n")
	fmt.Fprintf(&sb, "export type Host = %s & { props: %s | null };\n\n", host, props)
	sb.WriteString("export interface PropertyMap {\n\tforEach(cb: (value: unknown, key: string | number | symbol) => void): void;\n}\n\n")
	sb.WriteString("export const mounts: Array<(host: Host, props: HostProps, ctx: IslandContext) => void> = [];\n")
	sb.WriteString("export const updates: Array<(host: Host, props: HostProps, changed: PropertyMap) => void> = [];\n")
	sb.WriteString("export const unmounts: Array<(host: Host) => void> = [];\n\n")
	sb.WriteString("export type HostProps = ")
	sb.WriteString(props)
	sb.WriteString(" | null;\n\n")
	sb.WriteString("// ctx aborts and disposes what it registered when this mount unmounts.\n")
	sb.WriteString("export function onMount(fn: (host: Host, props: HostProps, ctx: IslandContext) => void): void {\n\tmounts.push(fn);\n}\n\n")
	sb.WriteString("export function onUpdate(fn: (host: Host, props: HostProps, changed: PropertyMap) => void): void {\n\tupdates.push(fn);\n}\n\n")
	sb.WriteString("export function onUnmount(fn: (host: Host) => void): void {\n\tunmounts.push(fn);\n}\n")
	if strings.TrimSpace(isl.Script) != "" {
		sb.WriteString("\n")
		sb.WriteString(strings.TrimSpace(isl.Script))
		sb.WriteString("\n")
	}
	return sb.String()
}

func baseSource(isl Island, base string) string {
	props := isl.PropsType
	lit := isl.Variant == "lit"
	tag := tsString(isl.Tag)
	var sb strings.Builder
	sb.WriteString(generatedHeader)
	sb.WriteString("\n")
	if lit {
		sb.WriteString("import { LitElement, type PropertyValues } from \"lit\";\n")
	}
	sb.WriteString("import { readProps } from \"../runtime/props\";\n")
	sb.WriteString("import { mountIsland, mounted, type Mounted } from \"../runtime/context\";\n")
	fmt.Fprintf(&sb, "import type { %s } from \"./%s.types\";\n", props, isl.Tag)
	fmt.Fprintf(&sb, "import * as hooks from \"./%s.hooks\";\n\n", isl.Tag)

	if lit {
		fmt.Fprintf(&sb, "export class %sBase extends LitElement {\n", base)
		sb.WriteString("\tstatic properties = { props: { type: Object } };\n")
		fmt.Fprintf(&sb, "\tdeclare props: %s | null;\n\n", props)
		sb.WriteString("\t// Light DOM so page CSS and the island's css chunk apply directly.\n")
		sb.WriteString("\tcreateRenderRoot(): HTMLElement | DocumentFragment {\n\t\treturn this;\n\t}\n\n")
		sb.WriteString("\t// lit inserts its nodes without removing the SSR placeholder children: clear them once.\n")
		sb.WriteString("\tdeclare _tcHydrated: boolean;\n")
	} else {
		fmt.Fprintf(&sb, "export class %sBase extends HTMLElement {\n", base)
		fmt.Fprintf(&sb, "\tdeclare props: %s | null;\n", props)
	}
	sb.WriteString("\tdeclare _tcMount: Mounted | undefined;\n\n")
	sb.WriteString("\tconnectedCallback(): void {\n")
	if lit {
		fmt.Fprintf(&sb, "\t\tthis.props = readProps<%s>(this) ?? this.props;\n", props)
		sb.WriteString("\t\tif (!this._tcHydrated) {\n")
		sb.WriteString("\t\t\tfor (const child of Array.from(this.children)) {\n")
		sb.WriteString("\t\t\t\tchild.remove();\n")
		sb.WriteString("\t\t\t}\n")
		sb.WriteString("\t\t\tthis._tcHydrated = true;\n")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t\tsuper.connectedCallback();\n")
	} else {
		fmt.Fprintf(&sb, "\t\tthis.props = readProps<%s>(this);\n", props)
	}
	fmt.Fprintf(&sb, "\t\tthis._tcMount?.dispose();\n\t\tconst m = (this._tcMount = mountIsland(%s, this));\n", tag)
	sb.WriteString("\t\tfor (const fn of hooks.mounts) fn(this, this.props, m.ctx);\n")
	if lit {
		fmt.Fprintf(&sb, "\t\tmounted(%s, this);\n\t}\n\n", tag)
		sb.WriteString("\tdisconnectedCallback(): void {\n\t\tsuper.disconnectedCallback();\n")
	} else {
		fmt.Fprintf(&sb, "\t\tmounted(%s, this);\n\t}\n\n", tag)
		sb.WriteString("\tdisconnectedCallback(): void {\n")
	}
	sb.WriteString("\t\tfor (const fn of hooks.unmounts) fn(this);\n")
	sb.WriteString("\t\tthis._tcMount?.dispose();\n\t\tthis._tcMount = undefined;\n\t}\n")
	if lit {
		sb.WriteString("\n\tprotected updated(changed: PropertyValues<this>): void {\n")
		sb.WriteString("\t\tsuper.updated(changed);\n")
		sb.WriteString("\t\tfor (const fn of hooks.updates) fn(this, this.props, changed);\n\t}\n\n")
		sb.WriteString("\tprotected render(): unknown {\n\t\treturn null;\n\t}\n")
		if len(isl.Binds) > 0 {
			sb.WriteString("\n")
			sb.WriteString("\t// A whole-props assignment trips the reactive property, so updated() runs per push.\n")
			sb.WriteString("\ttcBind(field: string, v: unknown): void {\n")
			fmt.Fprintf(&sb, "\t\tif (!this.props) this.props = {} as %s;\n", props)
			sb.WriteString("\t\tthis.props = { ...this.props, [field]: v };\n")
			sb.WriteString("\t}\n")
		}
	}
	sb.WriteString("}\n")
	return sb.String()
}

func defineSource(isl Island, base string) string {
	var sb strings.Builder
	sb.WriteString("// Written once; edit freely. Delete to re-scaffold.\n\n")
	fmt.Fprintf(&sb, "import { %sBase } from \"templatic/islands/%s.base\";\n\n", base, isl.Tag)
	fmt.Fprintf(&sb, "customElements.define(\n\t%s,\n\tclass extends %sBase {\n", tsString(isl.Tag), base)
	if isl.Variant == "lit" {
		sb.WriteString("\t\t// lit islands implement render() here.\n")
	}
	sb.WriteString("\t},\n);\n")
	return sb.String()
}

// tsString is s as a TS string literal; JSON strings are valid JS literals (invalid UTF-8 becomes U+FFFD).
func tsString(s string) string {
	b, err := json.Marshal(s, jsontext.AllowInvalidUTF8(true))
	if err != nil {
		panic(err)
	}
	return string(b)
}

func writeGenerated(path, content string) error {
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return writeAtomic(path, []byte(content))
}

func pascal(tag string) string {
	parts := strings.Split(tag, "-")
	var sb strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		sb.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return sb.String()
}
