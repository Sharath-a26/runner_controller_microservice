package util

import (
	"fmt"
	"slices"
)

func ValidateAlgorithmName(algo string) error {
	if slices.Contains([]string{"eaSimple", "eaMuPlusLambda", "eaMuCommaLambda", "eaGenerateUpdate", "de"}, algo) {
		return nil
	}
	return fmt.Errorf("invalid algorithm name: %s", algo)
}
