package thompsonsampling

import (
	"strings"
)

const ALGONamespace = "Tompsonsampling::"

func BuildKey(business_namespcae, target_namespacestring, candidate string) string {
	if business_namespcae == "" {
		business_namespcae = "__global__"
	}
	if target_namespacestring == "" {
		target_namespacestring = "__global__"
	}
	builder := strings.Builder{}
	builder.Grow(len(ALGONamespace) + len(business_namespcae) + len(target_namespacestring) + len(candidate) + 4)
	builder.WriteString(ALGONamespace)
	builder.WriteString(business_namespcae)
	builder.WriteString("::")
	builder.WriteString(target_namespacestring)
	builder.WriteString("::")
	builder.WriteString(candidate)
	return builder.String()
}
