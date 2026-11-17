package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/format"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/ardanlabs/kronk/sdk/kronk/model"
)

// Keep the agent example's four tool names and argument shapes. These local
// implementations bound output and use os.Root instead of unrestricted paths.
func toolDocuments() []model.D {
	path := model.D{"type": "string", "description": "Relative path inside the workspace; use . to search the workspace."}
	return []model.D{
		toolDocument("tool_read_file", "Read a UTF-8 file up to 64 KiB. Returns file_contents with 1-based line numbers for the Go editor.",
			model.D{"path": path}, "path"),
		toolDocument("tool_search_files", "Search workspace files by optional Go regex filename filter and content contains. Skips hidden directories, vendor and node_modules. Returns at most 100 paths; truncated means narrow the search. Content search skips files larger than 64 KiB.",
			model.D{"path": path, "filter": model.D{"type": "string", "description": "Go regex matching relative filenames; empty matches all."},
				"contains": model.D{"type": "string", "description": "Go regex matching file contents; empty disables content search."}}, "path"),
		toolDocument("tool_create_file", "Create an empty file and missing parent directories. Fails if the file exists. Requires approval.",
			model.D{"path": path}, "path"),
		toolDocument("tool_go_code_editor", "Add, replace or delete a line in a Go file, then gofmt it. Reads at most 64 KiB and rejects invalid Go. Requires approval; read the file first to obtain current line numbers.",
			model.D{"path": path, "line_number": model.D{"type": "integer", "description": "1-based line from tool_read_file; add inserts before this line."},
				"type_change": model.D{"type": "string", "enum": []string{"add", "replace", "delete"}},
				"line_change": model.D{"type": "string", "description": "Text to insert or replace; empty for delete. May contain newlines."}},
			"path", "line_number", "type_change", "line_change"),
	}
}

func toolDocument(name, description string, properties model.D, required ...string) model.D {
	return model.D{"type": "function", "function": model.D{"name": name, "description": description,
		"parameters": model.D{"type": "object", "properties": properties, "required": required}}}
}

func toolResponse(call model.ResponseToolCall, data any, err error) model.D {
	status := "SUCCESS"
	if err != nil {
		status, data = "FAILED", model.D{"error": err.Error()}
	}
	// All callers provide JSON-owned maps, strings and slices.
	content, encodeErr := json.Marshal(model.D{"status": status, "data": data})
	if encodeErr != nil {
		content = []byte(`{"status":"FAILED","data":{"error":"could not encode tool result"}}`)
	}
	return model.D{"role": "tool", "name": call.Function.Name, "tool_call_id": call.ID, "content": string(content)}
}

func callTool(ctx context.Context, root *os.Root, call model.ResponseToolCall) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var args struct {
		Path       string `json:"path"`
		Filter     string `json:"filter"`
		Contains   string `json:"contains"`
		LineNumber int    `json:"line_number"`
		TypeChange string `json:"type_change"`
		LineChange string `json:"line_change"`
	}
	// ToolCallArguments marshals as an OpenAI JSON string on the wire.
	// Convert to a plain map when decoding the argument object locally.
	data, err := json.Marshal(map[string]any(call.Function.Arguments))
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &args); err != nil {
		return nil, fmt.Errorf("invalid tool arguments: %w", err)
	}
	if args.Path == "" {
		args.Path = "."
	}
	if !filepath.IsLocal(args.Path) {
		return nil, errors.New("path must be relative and inside the workspace")
	}
	switch call.Function.Name {
	case "tool_read_file":
		data, err := readToolFile(root, args.Path)
		if err != nil {
			return nil, err
		}
		var numbered strings.Builder
		for i, line := range strings.Split(string(data), "\n") {
			fmt.Fprintf(&numbered, "%d: %s\n", i+1, line)
		}
		return model.D{"file_contents": numbered.String()}, nil

	case "tool_search_files":
		return searchFiles(ctx, root, args.Path, args.Filter, args.Contains)

	case "tool_create_file":
		if err := root.MkdirAll(filepath.Dir(args.Path), 0755); err != nil {
			return nil, err
		}
		f, err := root.OpenFile(args.Path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			return nil, err
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
		return model.D{"created": args.Path}, nil

	case "tool_go_code_editor":
		data, err := readToolFile(root, args.Path)
		if err != nil {
			return nil, err
		}
		if filepath.Ext(args.Path) != ".go" {
			return nil, errors.New("editor requires a .go file")
		}
		lines := strings.Split(string(data), "\n")
		if args.LineNumber < 1 || args.LineNumber > len(lines) {
			return nil, fmt.Errorf("line_number must be between 1 and %d", len(lines))
		}
		i := args.LineNumber - 1
		switch args.TypeChange {
		case "add":
			lines = slices.Insert(lines, i, args.LineChange)
		case "replace":
			lines[i] = args.LineChange
		case "delete":
			lines = append(lines[:i], lines[i+1:]...)
		default:
			return nil, errors.New("type_change must be add, replace or delete")
		}
		formatted, err := format.Source([]byte(strings.Join(lines, "\n")))
		if err != nil {
			return nil, fmt.Errorf("invalid Go; file was not changed: %w", err)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if err := root.WriteFile(args.Path, formatted, 0644); err != nil {
			return nil, err
		}
		return model.D{"edited": args.Path, "line_number": args.LineNumber, "type_change": args.TypeChange}, nil
	}
	return nil, fmt.Errorf("unknown tool %q", call.Function.Name)
}

func readToolFile(root *os.Root, path string) ([]byte, error) {
	info, err := root.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, errors.New("tool only reads regular files")
	}
	f, err := root.Open(path)
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(f, (64<<10)+1))
	if err := errors.Join(readErr, f.Close()); err != nil {
		return nil, err
	}
	if len(data) > 64<<10 {
		return nil, errors.New("file exceeds 64 KiB; choose a smaller file")
	}
	return data, nil
}

func searchFiles(ctx context.Context, root *os.Root, path, filter, contains string) (any, error) {
	names, err := regexp.Compile(filter)
	if err != nil {
		return nil, fmt.Errorf("invalid filename regex: %w", err)
	}
	contents, err := regexp.Compile(contains)
	if err != nil {
		return nil, fmt.Errorf("invalid content regex: %w", err)
	}
	var files []string
	var visited, skipped int
	var truncated bool
	err = fs.WalkDir(root.FS(), filepath.ToSlash(path), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		visited++
		if visited > 10000 || len(files) == 100 {
			truncated = true
			return fs.SkipAll
		}
		if entry.IsDir() {
			if path != "." && (strings.HasPrefix(entry.Name(), ".") || entry.Name() == "vendor" || entry.Name() == "node_modules") {
				return fs.SkipDir
			}
			return nil
		}
		if !entry.Type().IsRegular() || !names.MatchString(path) {
			return nil
		}
		if contains != "" {
			data, err := readToolFile(root, path)
			if err != nil {
				skipped++
				return nil
			}
			if !contents.Match(data) {
				return nil
			}
		}
		files = append(files, path)
		return nil
	})
	return model.D{"files": files, "truncated": truncated, "skipped": skipped}, err
}
