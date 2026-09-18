package policy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"

	"github.com/open-policy-agent/opa/v1/ast"
)

const capabilityProfileVersion = "meeseek-mvc-opa-safe-v1"

var safeBuiltins = []*ast.Builtin{
	ast.Equality,
	ast.Assign,
	ast.Member,
	ast.MemberWithKey,
	ast.GreaterThan,
	ast.GreaterThanEq,
	ast.LessThan,
	ast.LessThanEq,
	ast.NotEqual,
	ast.Equal,
	ast.Plus,
	ast.Minus,
	ast.Multiply,
	ast.Divide,
	ast.Ceil,
	ast.Floor,
	ast.Round,
	ast.Abs,
	ast.Rem,
	ast.And,
	ast.Or,
	ast.Count,
	ast.Sum,
	ast.Product,
	ast.Max,
	ast.Min,
	ast.Any,
	ast.All,
	ast.ArrayConcat,
	ast.ArrayFlatten,
	ast.ArraySlice,
	ast.ArrayReverse,
	ast.ToNumber,
	ast.SetDiff,
	ast.Intersection,
	ast.Union,
	ast.Concat,
	ast.IndexOf,
	ast.IndexOfN,
	ast.Substring,
	ast.Lower,
	ast.Upper,
	ast.Contains,
	ast.StartsWith,
	ast.EndsWith,
	ast.Split,
	ast.SplitN,
	ast.Replace,
	ast.Trim,
	ast.TrimSpace,
	ast.Sprintf,
	ast.JSONMarshal,
	ast.JSONUnmarshal,
	ast.JSONIsValid,
	ast.HexEncode,
	ast.HexDecode,
	ast.ObjectUnion,
	ast.ObjectRemove,
	ast.ObjectFilter,
	ast.ObjectGet,
	ast.ObjectKeys,
	ast.Sort,
	ast.IsNumber,
	ast.IsString,
	ast.IsBoolean,
	ast.IsArray,
	ast.IsObject,
	ast.IsSet,
	ast.IsNull,
}

func safeCapabilities() *ast.Capabilities {
	return &ast.Capabilities{
		Builtins: append([]*ast.Builtin(nil), safeBuiltins...),
		Features: []string{
			ast.FeatureRegoV1,
			ast.FeatureKeywordsInRefs,
			ast.FeatureTemplateStrings,
		},
		AllowNet: []string{},
	}
}

func SafeCapabilitiesHash() (string, error) {
	return capabilityProfileHash(safeCapabilities())
}

func capabilityProfileHash(caps *ast.Capabilities) (string, error) {
	builtinNames := make([]string, 0, len(caps.Builtins))
	for _, builtin := range caps.Builtins {
		builtinNames = append(builtinNames, builtin.Name)
	}
	sort.Strings(builtinNames)
	features := append([]string(nil), caps.Features...)
	sort.Strings(features)
	keywords := append([]string(nil), caps.FutureKeywords...)
	sort.Strings(keywords)

	descriptor := struct {
		Version        string   `json:"version"`
		Builtins       []string `json:"builtins"`
		Features       []string `json:"features"`
		FutureKeywords []string `json:"future_keywords"`
		Network        string   `json:"network"`
	}{
		Version:        capabilityProfileVersion,
		Builtins:       builtinNames,
		Features:       features,
		FutureKeywords: keywords,
		Network:        "DENY_ALL",
	}
	encoded, err := json.Marshal(descriptor)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}
