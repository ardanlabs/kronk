// Package prepare owns Moxie's raw-document preparation artifacts.
package prepare

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// DocumentExtractor is the document-conversion boundary used by Manager.
type DocumentExtractor interface {
	Convert(ctx context.Context, sourcePath string) (Document, error)
	ExtractionProfile() string
}

// Manager extracts raw documents and saves durable Moxie artifacts.
type Manager struct {
	extractor DocumentExtractor
	root      string
	now       func() time.Time
	random    io.Reader
}

// Artifact describes a document extraction stored by Moxie.
type Artifact struct {
	ID             string
	Directory      string
	SourcePath     string
	MarkdownPath   string
	StructuredPath string
	ManifestPath   string
	Cached         bool
}

// DatasetDocument identifies one extracted source within a managed dataset.
type DatasetDocument struct {
	ID        string
	Name      string
	Directory string
}

// New creates a manager that stores application data below ~/.moxie.
func New(extractor DocumentExtractor) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("locate home directory: %w", err)
	}

	return NewAt(extractor, filepath.Join(home, ".moxie"))
}

// NewAt creates a manager rooted at the specified application-data directory.
func NewAt(extractor DocumentExtractor, root string) (*Manager, error) {
	if extractor == nil {
		return nil, fmt.Errorf("new prepare manager: document extractor is required")
	}
	if root == "" {
		return nil, fmt.Errorf("new prepare manager: application-data directory is required")
	}

	return &Manager{
		extractor: extractor,
		root:      root,
		now:       time.Now,
		random:    rand.Reader,
	}, nil
}

// FindCachedExtraction returns a complete prior extraction for the exact
// source contents and format. Directories and base names do not affect cache
// identity.
func (m *Manager) FindCachedExtraction(ctx context.Context, sourcePath string) (Artifact, bool, error) {
	sourceHash, err := hashSource(ctx, sourcePath)
	if err != nil {
		return Artifact{}, false, err
	}

	artifact, found := m.findCachedExtraction(sourceHash, filepath.Ext(sourcePath))
	return artifact, found, nil
}

// Extract converts a raw document and stores its source and extracted forms.
func (m *Manager) Extract(ctx context.Context, sourcePath string) (Artifact, error) {
	sourceHash, err := hashSource(ctx, sourcePath)
	if err != nil {
		return Artifact{}, err
	}
	if artifact, found := m.findCachedExtraction(sourceHash, filepath.Ext(sourcePath)); found {
		artifact.Cached = true
		return artifact, nil
	}

	document, err := m.extractor.Convert(ctx, sourcePath)
	if err != nil {
		return Artifact{}, fmt.Errorf("extract source document: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return Artifact{}, fmt.Errorf("extract source document: %w", err)
	}

	id, err := m.newID()
	if err != nil {
		return Artifact{}, err
	}

	datasetsPath := filepath.Join(m.root, "datasets")
	if err := os.MkdirAll(datasetsPath, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create datasets directory: %w", err)
	}
	if err := os.Chmod(datasetsPath, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("secure datasets directory: %w", err)
	}

	temporaryPath, err := os.MkdirTemp(datasetsPath, ".building-")
	if err != nil {
		return Artifact{}, fmt.Errorf("create dataset workspace: %w", err)
	}
	keepTemporary := false
	defer func() {
		if !keepTemporary {
			_ = os.RemoveAll(temporaryPath)
		}
	}()

	sourceDirectory := filepath.Join(temporaryPath, "source")
	if err := os.Mkdir(sourceDirectory, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create source directory: %w", err)
	}

	storedSource := filepath.Join(sourceDirectory, filepath.Base(sourcePath))
	storedSourceHash, err := copySource(storedSource, sourcePath)
	if err != nil {
		return Artifact{}, err
	}
	if storedSourceHash != sourceHash {
		return Artifact{}, fmt.Errorf("store source document: source changed during extraction")
	}

	markdownPath := filepath.Join(temporaryPath, "extracted.md")
	if err := os.WriteFile(markdownPath, []byte(document.Markdown), 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write extracted Markdown: %w", err)
	}

	structured := document.StructuredJSON
	if len(structured) == 0 {
		structured = json.RawMessage(`{}`)
	}
	structuredPath := filepath.Join(temporaryPath, "extracted.docling.json")
	if err := os.WriteFile(structuredPath, append(structured, '\n'), 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write Docling JSON: %w", err)
	}

	createdAt := m.now().UTC()
	manifest := manifest{
		Version:              2,
		ID:                   id,
		CreatedAt:            createdAt,
		SourceName:           filepath.Base(sourcePath),
		SourceSHA256:         sourceHash,
		MarkdownSHA256:       hashBytes([]byte(document.Markdown)),
		StructuredSHA256:     hashBytes(append(append([]byte(nil), structured...), '\n')),
		Extractor:            "docling",
		ExtractionProfile:    m.extractor.ExtractionProfile(),
		ExtractionSeconds:    document.ProcessingTime,
		MarkdownFile:         "extracted.md",
		StructuredFile:       "extracted.docling.json",
		OriginalSourceFolder: "source",
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("encode dataset manifest: %w", err)
	}
	manifestPath := filepath.Join(temporaryPath, "manifest.json")
	if err := os.WriteFile(manifestPath, append(manifestData, '\n'), 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write dataset manifest: %w", err)
	}

	finalPath := filepath.Join(datasetsPath, id)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return Artifact{}, fmt.Errorf("publish dataset extraction: %w", err)
	}
	keepTemporary = true

	return Artifact{
		ID:             id,
		Directory:      finalPath,
		SourcePath:     filepath.Join(finalPath, "source", filepath.Base(sourcePath)),
		MarkdownPath:   filepath.Join(finalPath, "extracted.md"),
		StructuredPath: filepath.Join(finalPath, "extracted.docling.json"),
		ManifestPath:   filepath.Join(finalPath, "manifest.json"),
	}, nil
}

// ExtractDocuments converts source documents one at a time and publishes one
// collection that the existing preview and build pipeline can consume.
func (m *Manager) ExtractDocuments(ctx context.Context, sourcePaths []string, progress ExtractionProgressFunc) (Artifact, error) {
	if len(sourcePaths) == 0 {
		return Artifact{}, fmt.Errorf("extract documents: at least one source document is required")
	}

	artifacts := make([]Artifact, 0, len(sourcePaths))
	for index, sourcePath := range sourcePaths {
		if progress != nil {
			progress(ExtractionProgress{Index: index, Stage: "Extracting with Docling"})
		}
		artifact, err := m.Extract(ctx, sourcePath)
		if err != nil {
			if progress != nil {
				progress(ExtractionProgress{Index: index, Stage: "Extraction failed", Error: err.Error()})
			}
			return Artifact{}, fmt.Errorf("extract document %q: %w", filepath.Base(sourcePath), err)
		}
		artifacts = append(artifacts, artifact)
		if progress != nil {
			stage := "Extraction complete"
			if artifact.Cached {
				stage = "Using cached extraction"
			}
			progress(ExtractionProgress{Index: index, Stage: stage, Completed: true, Cached: artifact.Cached})
		}
	}

	if len(artifacts) == 1 {
		return artifacts[0], nil
	}
	return m.publishCollection(ctx, sourcePaths, artifacts)
}

type collectionDocument struct {
	SourceName string          `json:"source_name"`
	Document   json.RawMessage `json:"document"`
}

type collectionManifest struct {
	Version              int                        `json:"version"`
	ID                   string                     `json:"id"`
	CreatedAt            time.Time                  `json:"created_at"`
	Extractor            string                     `json:"extractor"`
	ExtractionProfile    string                     `json:"extraction_profile"`
	MarkdownSHA256       string                     `json:"markdown_sha256"`
	StructuredSHA256     string                     `json:"structured_sha256"`
	MarkdownFile         string                     `json:"markdown_file"`
	StructuredFile       string                     `json:"structured_file"`
	OriginalSourceFolder string                     `json:"original_source_folder"`
	DocumentsFolder      string                     `json:"documents_folder"`
	Sources              []collectionManifestSource `json:"sources"`
}

type collectionManifestSource struct {
	Name           string `json:"name"`
	SourceFile     string `json:"source_file"`
	SourceSHA256   string `json:"source_sha256"`
	MarkdownFile   string `json:"markdown_file"`
	StructuredFile string `json:"structured_file"`
}

// DatasetDocuments returns the independently extracted sources in a managed
// dataset. Version 2 single-document datasets remain one document rooted at
// the dataset directory; version 3 collections use their numbered document
// directories.
func DatasetDocuments(directory string) ([]DatasetDocument, error) {
	manifestPath := filepath.Join(directory, "manifest.json")
	data, err := os.ReadFile(manifestPath)
	if os.IsNotExist(err) {
		if _, markdownErr := os.Stat(filepath.Join(directory, "extracted.md")); markdownErr == nil {
			return []DatasetDocument{{ID: "001", Name: "Source document", Directory: directory}}, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("read dataset manifest: %w", err)
	}
	var header struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(data, &header); err != nil {
		return nil, fmt.Errorf("decode dataset manifest: %w", err)
	}

	switch header.Version {
	case 2:
		var single manifest
		if err := json.Unmarshal(data, &single); err != nil {
			return nil, fmt.Errorf("decode single-document manifest: %w", err)
		}
		if single.SourceName == "" {
			return nil, fmt.Errorf("decode single-document manifest: source_name is required")
		}
		if _, err := os.Stat(filepath.Join(directory, "extracted.md")); err != nil {
			return nil, fmt.Errorf("locate extracted Markdown for %q: %w", single.SourceName, err)
		}
		return []DatasetDocument{{ID: "001", Name: single.SourceName, Directory: directory}}, nil

	case 3:
		var collection collectionManifest
		if err := json.Unmarshal(data, &collection); err != nil {
			return nil, fmt.Errorf("decode collection manifest: %w", err)
		}
		if len(collection.Sources) == 0 {
			return nil, fmt.Errorf("decode collection manifest: sources are required")
		}
		documents := make([]DatasetDocument, len(collection.Sources))
		for index, source := range collection.Sources {
			id := fmt.Sprintf("%03d", index+1)
			documentDirectory := filepath.Join(directory, collection.DocumentsFolder, id)
			if source.Name == "" {
				return nil, fmt.Errorf("decode collection manifest: source %s name is required", id)
			}
			if filepath.Clean(source.MarkdownFile) != filepath.Join(collection.DocumentsFolder, id, "extracted.md") {
				return nil, fmt.Errorf("decode collection manifest: source %s Markdown path is invalid", id)
			}
			if _, err := os.Stat(filepath.Join(documentDirectory, "extracted.md")); err != nil {
				return nil, fmt.Errorf("locate extracted Markdown for %q: %w", source.Name, err)
			}
			documents[index] = DatasetDocument{ID: id, Name: source.Name, Directory: documentDirectory}
		}
		return documents, nil

	default:
		return nil, fmt.Errorf("decode dataset manifest: unsupported version %d", header.Version)
	}
}

func (m *Manager) publishCollection(ctx context.Context, sourcePaths []string, artifacts []Artifact) (Artifact, error) {
	id, err := m.newID()
	if err != nil {
		return Artifact{}, err
	}
	datasetsPath := filepath.Join(m.root, "datasets")
	if err := os.MkdirAll(datasetsPath, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("create datasets directory: %w", err)
	}
	if err := os.Chmod(datasetsPath, 0o700); err != nil {
		return Artifact{}, fmt.Errorf("secure datasets directory: %w", err)
	}

	temporaryPath, err := os.MkdirTemp(datasetsPath, ".building-")
	if err != nil {
		return Artifact{}, fmt.Errorf("create dataset collection workspace: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(temporaryPath)
		}
	}()

	sourceDirectory := filepath.Join(temporaryPath, "source")
	documentsDirectory := filepath.Join(temporaryPath, "documents")
	for _, directory := range []string{sourceDirectory, documentsDirectory} {
		if err := os.Mkdir(directory, 0o700); err != nil {
			return Artifact{}, fmt.Errorf("create dataset collection directory: %w", err)
		}
	}

	var markdown strings.Builder
	structuredDocuments := make([]collectionDocument, 0, len(artifacts))
	sources := make([]collectionManifestSource, 0, len(artifacts))
	for index, artifact := range artifacts {
		if err := ctx.Err(); err != nil {
			return Artifact{}, fmt.Errorf("publish document collection: %w", err)
		}

		number := fmt.Sprintf("%03d", index+1)
		sourceName := filepath.Base(sourcePaths[index])
		storedSourceName := number + "-" + sourceName
		storedSource := filepath.Join(sourceDirectory, storedSourceName)
		sourceHash, err := copySource(storedSource, artifact.SourcePath)
		if err != nil {
			return Artifact{}, err
		}

		documentDirectory := filepath.Join(documentsDirectory, number)
		if err := os.Mkdir(documentDirectory, 0o700); err != nil {
			return Artifact{}, fmt.Errorf("create collection document directory: %w", err)
		}
		documentMarkdown, err := os.ReadFile(artifact.MarkdownPath)
		if err != nil {
			return Artifact{}, fmt.Errorf("read extracted Markdown for %q: %w", sourceName, err)
		}
		documentStructured, err := os.ReadFile(artifact.StructuredPath)
		if err != nil {
			return Artifact{}, fmt.Errorf("read structured extraction for %q: %w", sourceName, err)
		}
		documentStructured = bytes.TrimSpace(documentStructured)
		if !json.Valid(documentStructured) {
			return Artifact{}, fmt.Errorf("read structured extraction for %q: invalid JSON", sourceName)
		}

		markdownRelative := filepath.Join("documents", number, "extracted.md")
		structuredRelative := filepath.Join("documents", number, "extracted.docling.json")
		if err := os.WriteFile(filepath.Join(temporaryPath, markdownRelative), documentMarkdown, 0o600); err != nil {
			return Artifact{}, fmt.Errorf("write collection Markdown for %q: %w", sourceName, err)
		}
		if err := os.WriteFile(filepath.Join(temporaryPath, structuredRelative), append(documentStructured, '\n'), 0o600); err != nil {
			return Artifact{}, fmt.Errorf("write collection structured extraction for %q: %w", sourceName, err)
		}

		if markdown.Len() > 0 {
			markdown.WriteString("\n\n")
		}
		fmt.Fprintf(&markdown, "# Source Document: %s\n\n%s", sourceName, strings.TrimSpace(string(documentMarkdown)))
		structuredDocuments = append(structuredDocuments, collectionDocument{SourceName: sourceName, Document: json.RawMessage(documentStructured)})
		sources = append(sources, collectionManifestSource{
			Name: sourceName, SourceFile: filepath.Join("source", storedSourceName), SourceSHA256: sourceHash,
			MarkdownFile: markdownRelative, StructuredFile: structuredRelative,
		})
	}

	markdownData := []byte(markdown.String())
	structuredData, err := json.MarshalIndent(struct {
		Documents []collectionDocument `json:"documents"`
	}{Documents: structuredDocuments}, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("encode collection structured extraction: %w", err)
	}
	structuredData = append(structuredData, '\n')
	markdownPath := filepath.Join(temporaryPath, "extracted.md")
	structuredPath := filepath.Join(temporaryPath, "extracted.docling.json")
	if err := os.WriteFile(markdownPath, markdownData, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write collection Markdown: %w", err)
	}
	if err := os.WriteFile(structuredPath, structuredData, 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write collection structured extraction: %w", err)
	}

	manifest := collectionManifest{
		Version: 3, ID: id, CreatedAt: m.now().UTC(), Extractor: "docling", ExtractionProfile: m.extractor.ExtractionProfile(),
		MarkdownSHA256: hashBytes(markdownData), StructuredSHA256: hashBytes(structuredData),
		MarkdownFile: "extracted.md", StructuredFile: "extracted.docling.json",
		OriginalSourceFolder: "source", DocumentsFolder: "documents", Sources: sources,
	}
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Artifact{}, fmt.Errorf("encode collection manifest: %w", err)
	}
	manifestPath := filepath.Join(temporaryPath, "manifest.json")
	if err := os.WriteFile(manifestPath, append(manifestData, '\n'), 0o600); err != nil {
		return Artifact{}, fmt.Errorf("write collection manifest: %w", err)
	}

	finalPath := filepath.Join(datasetsPath, id)
	if err := os.Rename(temporaryPath, finalPath); err != nil {
		return Artifact{}, fmt.Errorf("publish dataset collection: %w", err)
	}
	published = true
	return Artifact{
		ID: id, Directory: finalPath, SourcePath: filepath.Join(finalPath, sources[0].SourceFile),
		MarkdownPath: filepath.Join(finalPath, "extracted.md"), StructuredPath: filepath.Join(finalPath, "extracted.docling.json"),
		ManifestPath: filepath.Join(finalPath, "manifest.json"),
	}, nil
}

type manifest struct {
	Version              int       `json:"version"`
	ID                   string    `json:"id"`
	CreatedAt            time.Time `json:"created_at"`
	SourceName           string    `json:"source_name"`
	SourceSHA256         string    `json:"source_sha256"`
	MarkdownSHA256       string    `json:"markdown_sha256"`
	StructuredSHA256     string    `json:"structured_sha256"`
	Extractor            string    `json:"extractor"`
	ExtractionProfile    string    `json:"extraction_profile,omitempty"`
	ExtractionSeconds    float64   `json:"extraction_seconds"`
	MarkdownFile         string    `json:"markdown_file"`
	StructuredFile       string    `json:"structured_file"`
	OriginalSourceFolder string    `json:"original_source_folder"`
}

func (m *Manager) findCachedExtraction(sourceHash, sourceExtension string) (Artifact, bool) {
	datasetsPath := filepath.Join(m.root, "datasets")
	entries, err := os.ReadDir(datasetsPath)
	if err != nil {
		return Artifact{}, false
	}

	for _, entry := range slices.Backward(entries) {

		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".building-") {
			continue
		}
		directory := filepath.Join(datasetsPath, entry.Name())
		manifestPath := filepath.Join(directory, "manifest.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue
		}
		var previous manifest
		if json.Unmarshal(data, &previous) != nil ||
			previous.ID != entry.Name() ||
			previous.SourceSHA256 != sourceHash ||
			!strings.EqualFold(filepath.Ext(previous.SourceName), sourceExtension) ||
			!compatibleExtractionProfile(previous, m.extractor.ExtractionProfile()) {
			continue
		}

		artifact := Artifact{
			ID:             previous.ID,
			Directory:      directory,
			SourcePath:     filepath.Join(directory, "source", previous.SourceName),
			MarkdownPath:   filepath.Join(directory, "extracted.md"),
			StructuredPath: filepath.Join(directory, "extracted.docling.json"),
			ManifestPath:   manifestPath,
		}
		if validCachedArtifact(artifact, previous) {
			return artifact, true
		}
	}

	return Artifact{}, false
}

func compatibleExtractionProfile(previous manifest, profile string) bool {
	return previous.Extractor == "docling" &&
		previous.Version == 2 &&
		previous.ExtractionProfile == profile
}

func validCachedArtifact(artifact Artifact, previous manifest) bool {
	if previous.SourceName == "" || filepath.Base(previous.SourceName) != previous.SourceName ||
		previous.MarkdownFile != "extracted.md" ||
		previous.StructuredFile != "extracted.docling.json" ||
		previous.OriginalSourceFolder != "source" {
		return false
	}

	for _, path := range []string{artifact.SourcePath, artifact.MarkdownPath, artifact.StructuredPath, artifact.ManifestPath} {
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
			return false
		}
	}
	sourceHash, sourceErr := hashFile(artifact.SourcePath)
	markdownHash, markdownErr := hashFile(artifact.MarkdownPath)
	structuredHash, structuredErr := hashFile(artifact.StructuredPath)
	return sourceErr == nil && markdownErr == nil && structuredErr == nil &&
		sourceHash == previous.SourceSHA256 &&
		markdownHash == previous.MarkdownSHA256 &&
		structuredHash == previous.StructuredSHA256
}

func (m *Manager) newID() (string, error) {
	random := make([]byte, 4)
	if _, err := io.ReadFull(m.random, random); err != nil {
		return "", fmt.Errorf("create dataset id: %w", err)
	}

	return m.now().UTC().Format("20060102T150405.000000000Z") + "-" + hex.EncodeToString(random), nil
}

func copySource(destination string, sourcePath string) (string, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open source document: %w", err)
	}
	defer source.Close()

	destinationFile, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("create stored source document: %w", err)
	}

	hash := sha256.New()
	_, copyErr := io.Copy(io.MultiWriter(destinationFile, hash), source)
	closeErr := destinationFile.Close()
	if copyErr != nil {
		return "", fmt.Errorf("store source document: %w", copyErr)
	}
	if closeErr != nil {
		return "", fmt.Errorf("close stored source document: %w", closeErr)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashSource(ctx context.Context, sourcePath string) (string, error) {
	source, err := os.Open(sourcePath)
	if err != nil {
		return "", fmt.Errorf("open source document: %w", err)
	}
	defer source.Close()

	hash := sha256.New()
	buffer := make([]byte, 128<<10)
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("hash source document: %w", err)
		}
		count, readErr := source.Read(buffer)
		if count > 0 {
			if _, err := hash.Write(buffer[:count]); err != nil {
				return "", fmt.Errorf("hash source document: %w", err)
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return "", fmt.Errorf("hash source document: %w", readErr)
		}
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func hashBytes(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}
