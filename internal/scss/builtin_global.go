// SPDX-License-Identifier: BSD-3-Clause
// Copyright (c) 2026, the go-scss/scss authors

package scss

// globalFns maps the un-namespaced (global) built-in function names available
// without @use, mirroring dart-sass's global function set.
var globalFns = map[string]builtinFunc{}

func init() {
	registerGlobals()
}

func registerGlobals() {
	m := map[string]builtinFunc{
		// colors
		"rgb":            fnRGB,
		"rgba":           fnRGB,
		"hsl":            fnHSL,
		"hsla":           fnHSL,
		"hwb":            fnHWB,
		"color":          fnColorFunc,
		"lab":            spaceFuncMaker(spaceLab, "lab"),
		"lch":            spaceFuncMaker(spaceLCH, "lch"),
		"oklab":          spaceFuncMaker(spaceOklab, "oklab"),
		"oklch":          spaceFuncMaker(spaceOklch, "oklch"),
		"red":            fnRed,
		"green":          fnGreen,
		"blue":           fnBlue,
		"hue":            fnHue,
		"saturation":     fnSaturation,
		"lightness":      fnLightness,
		"whiteness":      fnWhiteness,
		"blackness":      fnBlackness,
		"alpha":          fnAlpha,
		"opacity":        fnAlpha,
		"mix":            fnMix,
		"grayscale":      fnGrayscale,
		"invert":         fnInvert,
		"complement":     fnComplement,
		"lighten":        fnLighten,
		"darken":         fnDarken,
		"saturate":       fnSaturate,
		"desaturate":     fnDesaturate,
		"adjust-hue":     fnAdjustHue,
		"opacify":        fnOpacify,
		"fade-in":        fnOpacify,
		"transparentize": fnTransparentize,
		"fade-out":       fnTransparentize,
		"adjust-color":   fnColorAdjust,
		"scale-color":    fnColorScale,
		"change-color":   fnColorChange,
		"ie-hex-str":     fnIEHexStr,

		// math
		"percentage": mathFns["percentage"],
		"round":      mathFns["round"],
		"ceil":       mathFns["ceil"],
		"floor":      mathFns["floor"],
		"abs":        mathFns["abs"],
		"min":        mathFns["min"],
		"max":        mathFns["max"],
		"unit":       mathFns["unit"],
		"unitless":   mathFns["is-unitless"],
		"comparable": mathFns["compatible"],

		// strings
		"quote":         stringFns["quote"],
		"unquote":       stringFns["unquote"],
		"str-length":    stringFns["length"],
		"str-insert":    stringFns["insert"],
		"str-index":     stringFns["index"],
		"str-slice":     stringFns["slice"],
		"to-upper-case": stringFns["to-upper-case"],
		"to-lower-case": stringFns["to-lower-case"],
		"unique-id":     stringFns["unique-id"],

		// lists
		"length":         listFns["length"],
		"nth":            listFns["nth"],
		"set-nth":        listFns["set-nth"],
		"join":           listFns["join"],
		"append":         listFns["append"],
		"zip":            listFns["zip"],
		"index":          listFns["index"],
		"list-separator": listFns["separator"],
		"is-bracketed":   listFns["is-bracketed"],

		// maps
		"map-get":     mapFns["get"],
		"map-merge":   mapFns["merge"],
		"map-remove":  mapFns["remove"],
		"map-keys":    mapFns["keys"],
		"map-values":  mapFns["values"],
		"map-has-key": mapFns["has-key"],

		// selectors
		"selector-nest":   selectorFns["nest"],
		"selector-append": selectorFns["append"],
		"selector-unify":  selectorFns["unify"],

		// meta
		"type-of":                metaFns["type-of"],
		"inspect":                metaFns["inspect"],
		"keywords":               metaFns["keywords"],
		"feature-exists":         metaFns["feature-exists"],
		"variable-exists":        metaFns["variable-exists"],
		"global-variable-exists": metaFns["global-variable-exists"],
		"function-exists":        metaFns["function-exists"],
		"mixin-exists":           metaFns["mixin-exists"],
		"content-exists":         metaFns["content-exists"],
	}
	for k, v := range m {
		globalFns[k] = v
	}
}
