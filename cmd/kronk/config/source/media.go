package source

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"github.com/ardanlabs/kronk/sdk/applog"
	buckymodels "github.com/ardanlabs/kronk/sdk/tools/bucky/models"
	malinamodels "github.com/ardanlabs/kronk/sdk/tools/malina/models"
	"github.com/ardanlabs/kronk/sdk/tools/modelconfig"
)

// buckyVAD is the voice activity detector in the whisper catalog. It pairs
// with a whisper model and has no entry of its own, so it is not offered.
const buckyVAD = "silero-vad"

// mediaChoices lists the audio and image models: the installed ones, and the
// catalog ones that are not.
func (s *Source) mediaChoices(doc modelconfig.Document) (installed []Choice, catalog []Choice) {
	seen := map[string]bool{}

	if mdls, err := buckymodels.NewWithPaths(s.basePath); err == nil && mdls.BuildIndex(applog.DiscardLogger, false) == nil {
		files, _ := mdls.Files()
		for _, f := range files {
			_, inFile := doc.BuckyModels[f.ID]
			seen[f.ID] = true
			installed = append(installed, Choice{ID: f.ID, Kind: KindAudioModel, Installed: true, Validated: true, InFile: inFile, Size: f.Size})
		}
	}
	for name, entry := range buckymodels.Catalog() {
		if seen[name] || name == buckyVAD {
			continue
		}
		_, inFile := doc.BuckyModels[name]
		catalog = append(catalog, Choice{ID: name, Kind: KindAudioModel, InFile: inFile, Size: parseSize(entry.Size)})
	}

	mdls, err := malinamodels.NewWithPaths(s.basePath)
	ready := err == nil && mdls.BuildIndex(applog.DiscardLogger, false) == nil
	for _, b := range malinamodels.Catalog() {
		name := b.Name.String()
		_, inFile := doc.MalinaModels[name]
		if ready {
			if mp, err := mdls.FullPath(name); err == nil {
				var size int64
				for _, n := range mp.FileSizes {
					size += n
				}
				installed = append(installed, Choice{ID: name, Kind: KindImageModel, Installed: true, Validated: true, InFile: inFile, Size: size})
				continue
			}
		}
		var size int64
		for _, f := range b.Files {
			size += parseSize(f.Size)
		}
		catalog = append(catalog, Choice{ID: name, Kind: KindImageModel, InFile: inFile, Size: size})
	}

	byKind := func(a, b Choice) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	}
	slices.SortFunc(installed, byKind)
	slices.SortFunc(catalog, byKind)

	return installed, catalog
}

// parseSize reads a catalog size such as "75 MB" or "1.5 GB", 0 when it
// can't. The units match FormatBytes, so the picker shows the catalog's
// number.
func parseSize(s string) int64 {
	num, unit, _ := strings.Cut(strings.TrimSpace(s), " ")
	n, err := strconv.ParseFloat(strings.TrimPrefix(num, "~"), 64)
	if err != nil {
		return 0
	}
	switch strings.ToUpper(unit) {
	case "GB":
		return int64(n * (1 << 30))
	case "MB":
		return int64(n * (1 << 20))
	}
	return 0
}
