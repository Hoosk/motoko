package tools

import "encoding/json"

const (
	schemaKeyType               = "type"
	schemaKeyObject             = "object"
	schemaKeyProperties         = "properties"
	schemaKeyAdditionalProps    = "additionalProperties"
	schemaKeyDescription        = "description"
	schemaKeyRequired           = "required"
)

// jsonSchema renders a JSON Schema (draft 2020-12) from a properties map and
// a required list. Tools that expose a schema get their arguments validated
// and described natively by the LLM providers instead of the synthetic
// {"input": string} fallback.
func jsonSchema(properties map[string]any, required ...string) []byte {
	schema := map[string]any{
		schemaKeyType:       schemaKeyObject,
		schemaKeyProperties: properties,
		schemaKeyAdditionalProps: false,
	}
	if len(required) > 0 {
		schema[schemaKeyRequired] = required
	}
	out, err := json.Marshal(schema)
	if err != nil {
		return nil
	}
	return out
}

// strProp builds a string property with an optional description.
func strProp(desc string) map[string]any {
	return map[string]any{schemaKeyType: "string", schemaKeyDescription: desc}
}

// intProp builds an integer property.
func intProp(desc string) map[string]any {
	return map[string]any{schemaKeyType: "integer", schemaKeyDescription: desc}
}

// schemaProps is a small DSL that reads naturally at call sites.
func schemaProps(pairs ...any) map[string]any {
	out := make(map[string]any, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		key, ok := pairs[i].(string)
		if !ok {
			continue
		}
		out[key] = pairs[i+1]
	}
	return out
}

var (
	schemaRead = jsonSchema(schemaProps(
		"path", strProp("File path (or directory) to read, relative to the workspace."),
		"offset", intProp("1-based line number to start reading from."),
		"limit", intProp("Maximum number of lines to read."),
	), "path")

	schemaWrite = jsonSchema(schemaProps(
		"path", strProp("File path to create or overwrite, relative to the workspace."),
		"content", strProp("Full file content to write."),
	), "path", "content")

	schemaBash = jsonSchema(schemaProps(
		"command", strProp("Shell command to run in the workspace."),
	), "command")

	schemaGrep = jsonSchema(schemaProps(
		"pattern", strProp("Regular expression to search for."),
		"regex", strProp("Alias for pattern."),
		"include", strProp("Optional glob to restrict files (e.g. *.go)."),
		"path", strProp("Optional directory to search, relative to the workspace."),
	), "pattern")

	schemaGlob = jsonSchema(schemaProps(
		"pattern", strProp("Glob pattern to match files (e.g. **/*.go)."),
	), "pattern")

	schemaInspect = jsonSchema(schemaProps(
		"worker", strProp("Tachikoma worker name to inspect."),
		"worker_name", strProp("Alias for worker."),
	), "worker")

	schemaWebFetch = jsonSchema(schemaProps(
		"url", strProp("URL to download and extract as readable text."),
	), "url")

	schemaWebSearch = jsonSchema(schemaProps(
		"query", strProp("Search query."),
		"search", strProp("Alias for query."),
	), "query")

	schemaTask = jsonSchema(schemaProps(
		"command", strProp("Command to run in the background."),
		"terminate", strProp("When true, terminate the task with the given id."),
		"task_id", strProp("Task id to terminate."),
		"id", strProp("Alias for task_id."),
	))

	schemaActivateSkill = jsonSchema(schemaProps(
		"name", strProp("Skill name from the catalog."),
		"skill", strProp("Alias for name."),
		"skill_name", strProp("Alias for name."),
	), "name")

	schemaQuestion = jsonSchema(schemaProps(
		"header", strProp("Short header shown above the question."),
		"question", strProp("Question text presented to the user."),
		"options", map[string]any{
			schemaKeyType: "array",
			"items": map[string]any{
				schemaKeyType: schemaKeyObject,
				schemaKeyProperties: schemaProps(
					"label", strProp("Option label."),
					"description", strProp("Optional explanation."),
				),
				schemaKeyRequired:             []string{"label"},
				schemaKeyAdditionalProps: false,
			},
			schemaKeyDescription: "Answer options; omit for free-text questions.",
		},
		"multiple", map[string]any{schemaKeyType: "boolean", "description": "Allow multiple selections."},
		"allow_custom", map[string]any{schemaKeyType: "boolean", "description": "Allow a custom answer."},
	), "question")

	schemaDelegate = jsonSchema(schemaProps(
		"agent", strProp("Target agent name (plan, search, ...)."),
		"instruction", strProp("Instruction for the sub-agent."),
	), "agent", "instruction")

	schemaBrainWrite = jsonSchema(schemaProps(
		"filename", strProp("Brain file name."),
		"content", strProp("File content."),
	), "filename", "content")

	schemaBrainRead = jsonSchema(schemaProps(
		"filename", strProp("Brain file name to read."),
		"file", strProp("Alias for filename."),
	), "filename")

	schemaBrainList = jsonSchema(nil)

	schemaPatch = jsonSchema(schemaProps(
		"path", strProp("File to patch, relative to the workspace."),
		"edits", map[string]any{
			"type":        "array",
			schemaKeyDescription: "List of edits: [{old, new}] or [{old_string, new_string}].",
			"items": map[string]any{
				schemaKeyType: schemaKeyObject,
				schemaKeyProperties: schemaProps(
					"old", strProp("Exact text to replace."),
					"old_string", strProp("Alias for old."),
					"new", strProp("Replacement text."),
					"new_string", strProp("Alias for new."),
				),
				schemaKeyRequired:             []string{"old", "new"},
				schemaKeyAdditionalProps: false,
			},
		},
	), "path", "edits")
)
