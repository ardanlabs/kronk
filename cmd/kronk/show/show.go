// Package show provides the show command code.
package show

import (
	"fmt"

	"github.com/ardanlabs/kronk/defaults"
	"github.com/ardanlabs/kronk/tools"
)

// RunLocal executes the pull command.
func RunLocal(args []string) error {
	libPath := defaults.LibsDir("")
	modelPath := defaults.ModelsDir("")
	modelName := args[0]

	mi, err := tools.ShowModel(libPath, modelPath, modelName)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Printf("ID:          %s\n", mi.ID)
	fmt.Printf("Desc:        %s\n", mi.Desc)
	fmt.Printf("Size:        %.2f MiB\n", float64(mi.Size)/(1024*1024))
	fmt.Printf("HasProj:     %t\n", mi.HasProjection)
	fmt.Printf("HasEncoder:  %t\n", mi.HasEncoder)
	fmt.Printf("HasDecoder:  %t\n", mi.HasDecoder)
	fmt.Printf("IsRecurrent: %t\n", mi.IsRecurrent)
	fmt.Printf("IsHybrid:    %t\n", mi.IsHybrid)
	fmt.Printf("IsGPT:       %t\n", mi.IsGPT)
	fmt.Println("Metadata:")
	for k, v := range mi.Metadata {
		fmt.Printf("  %s: %s\n", k, v)
	}

	return nil
}
