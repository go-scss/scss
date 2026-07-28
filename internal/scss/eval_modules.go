// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

import (
	"path"
	"strings"
)

var builtinModuleNames = map[string]bool{
	"math": true, "color": true, "string": true, "list": true,
	"map": true, "selector": true, "meta": true,
}

func (e *evaluator) evalUse(n *Use) {
	ns := e.namespaceFor(n.URL, n.Namespace)
	if strings.HasPrefix(n.URL, "sass:") {
		modName := strings.TrimPrefix(n.URL, "sass:")
		if !builtinModuleNames[modName] {
			e.fail("Can't find module \"%s\".", n.URL)
		}
		if n.NoNS {
			e.env.builtinGlobals = append(e.env.builtinGlobals, modName)
		} else {
			if e.env.builtinAliases == nil {
				e.env.builtinAliases = map[string]string{}
			}
			e.env.builtinAliases[ns] = modName
		}
		return
	}
	mod := e.loadModule(n.URL, n.Config)
	if n.NoNS {
		e.mergeModuleGlobally(mod, "")
	} else {
		e.env.modules[ns] = mod
	}
}

func (e *evaluator) namespaceFor(url, explicit string) string {
	if explicit != "" {
		return explicit
	}
	base := url
	if strings.HasPrefix(base, "sass:") {
		return strings.TrimPrefix(base, "sass:")
	}
	base = path.Base(base)
	base = strings.TrimSuffix(base, ".scss")
	base = strings.TrimSuffix(base, ".sass")
	base = strings.TrimPrefix(base, "_")
	return base
}

func (e *evaluator) evalForward(n *Forward) {
	if strings.HasPrefix(n.URL, "sass:") {
		return
	}
	mod := e.loadModule(n.URL, n.Config)
	e.mergeModuleGlobally(mod, n.Prefix)
	// track for downstream @use of THIS stylesheet
	e.forwarded = append(e.forwarded, forwardedMod{mod: mod, prefix: n.Prefix})
}

func (e *evaluator) mergeModuleGlobally(mod *module, prefix string) {
	for k, v := range mod.vars {
		e.env.setGlobalIfAbsent(prefix+k, v)
	}
	for k, v := range mod.mixins {
		e.env.mixins[normIdent(prefix+k)] = v
	}
	for k, v := range mod.funcs {
		e.env.funcs[normIdent(prefix+k)] = v
	}
}

type forwardedMod struct {
	mod    *module
	prefix string
}

func (e *evaluator) loadModule(url string, config []ConfigVar) *module {
	if m, ok := e.loaded[url]; ok {
		return m
	}
	src, resolved, ok := e.resolve(url)
	if !ok {
		e.fail("Can't find stylesheet to import: %s", url)
	}
	for _, s := range e.loadStack {
		if s == resolved {
			e.fail("Module loop: %s", resolved)
		}
	}
	stmts, err := parseStylesheet(src)
	if err != nil {
		panic(err)
	}
	sub := newEvaluator(e.importer)
	sub.loadStack = append(append([]string(nil), e.loadStack...), resolved)
	sub.loadedURLs = e.loadedURLs
	// apply configuration
	for _, c := range config {
		sub.env.scopes[0][c.Name] = e.evalExpr(c.Value)
	}
	sub.runModule(stmts)
	e.warnings = append(e.warnings, sub.warnings...)
	e.loadedURLs = append(e.loadedURLs, resolved)
	// include used module CSS in our output
	for _, node := range sub.root.children() {
		e.root.appendNode(node)
	}
	mod := &module{
		vars:   sub.env.scopes[0],
		mixins: sub.env.mixins,
		funcs:  sub.env.funcs,
		env:    sub.env,
	}
	// include forwarded members
	for _, fw := range sub.forwarded {
		for k, v := range fw.mod.vars {
			mod.vars[fw.prefix+k] = v
		}
		for k, v := range fw.mod.mixins {
			mod.mixins[fw.prefix+k] = v
		}
		for k, v := range fw.mod.funcs {
			mod.funcs[fw.prefix+k] = v
		}
	}
	e.loaded[url] = mod
	return mod
}

func (e *evaluator) runModule(stmts []Stmt) {
	fr := &frame{container: e.root, rootContainer: e.root, mediaParent: e.root, atContainer: true, group: &groupInfo{}}
	e.evalBody(stmts, fr, true)
	e.applyExtends()
}

func (e *evaluator) resolve(url string) (string, string, bool) {
	if e.importer == nil {
		return "", "", false
	}
	return e.importer(url)
}

func (e *evaluator) evalImport(n *Import, fr *frame) {
	for _, item := range n.Imports {
		if item.Plain {
			params := "\"" + item.URL + "\""
			if strings.HasPrefix(item.URL, "url(") || strings.HasPrefix(item.RawText, "url") {
				params = item.URL
			}
			if item.RawText != "" {
				params += " " + item.RawText
			}
			at := &cssAtRule{name: "import", params: params, hasBody: false}
			at.blankBefore = e.consumeGroup(fr)
			fr.container.appendNode(at)
			continue
		}
		src, _, ok := e.resolve(item.URL)
		if !ok {
			// treat as plain CSS import passthrough
			at := &cssAtRule{name: "import", params: "\"" + item.URL + "\"", hasBody: false}
			at.blankBefore = e.consumeGroup(fr)
			fr.container.appendNode(at)
			continue
		}
		stmts, err := parseStylesheet(src)
		if err != nil {
			panic(err)
		}
		e.evalBody(stmts, fr, true)
	}
}
