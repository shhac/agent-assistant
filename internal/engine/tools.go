package engine

// Tool argument types are shared with the application's strict, policy-checking bridge.
type CreateProjectArgs struct {
	Title              string   `json:"title"`
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type UpdateProjectArgs struct {
	ProjectID          string   `json:"project_id"`
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}

type DelegateArgs struct {
	ProjectID          string   `json:"project_id"`
	ParentID           string   `json:"parent_id"`
	WorkerProfile      string   `json:"worker_profile"`
	Role               string   `json:"role"`
	Objective          string   `json:"objective"`
	AcceptanceCriteria []string `json:"acceptance_criteria"`
}
type DecisionArgs struct {
	ProjectID      string   `json:"project_id"`
	Question       string   `json:"question"`
	Recommendation string   `json:"recommendation"`
	Options        []string `json:"options"`
	Why            string   `json:"why"`
	Evidence       []string `json:"evidence"`
}
type PreferenceArgs struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type MessageAgentArgs struct {
	AgentID string `json:"agent_id"`
	Message string `json:"message"`
}

type CompleteProjectArgs struct {
	ProjectID string   `json:"project_id"`
	Evidence  []string `json:"evidence"`
}

type StatusArgs struct {
	ProjectID string   `json:"project_id"`
	Summary   string   `json:"summary"`
	Evidence  []string `json:"evidence"`
}

type Tool struct {
	Type     string   `json:"type"`
	Function Function `json:"function"`
}
type Function struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
	Strict      bool           `json:"strict"`
}

func Tools() []Tool {
	return []Tool{
		tool("read_state", "Read current projects, work, decisions, preferences, available profiles and authority.", nil, nil),
		tool("create_project", "Record a project outcome and evidence required for acceptance. This creates coordination metadata only.", []string{"title", "objective"}, []string{"acceptance_criteria"}),
		tool("update_project", "Refine an uncommissioned project brief into concrete acceptance criteria before delegating. Cannot change the acceptance contract after workers are commissioned.", []string{"project_id", "objective"}, []string{"acceptance_criteria"}),
		tool("delegate", "Commission an approved worker or manager. Use an empty parent_id when reporting directly to the PA. The daemon enforces inherited scope and limits.", []string{"project_id", "parent_id", "worker_profile", "role", "objective"}, []string{"acceptance_criteria"}),
		tool("ask_decision", "Prepare an unresolved owner decision. Include recommendation, viable alternatives, consequences and evidence.", []string{"project_id", "question", "recommendation", "why"}, []string{"options", "evidence"}),
		tool("remember_preference", "Remember an owner preference. This cannot grant permissions or change budgets.", []string{"key", "value"}, nil),
		tool("message_agent", "Send a coordination instruction or answer to an existing agent. Does not grant new authority.", []string{"agent_id", "message"}, nil),
		tool("complete_project", "Accept a completed project only after all descendant work is finished and evidence satisfies the acceptance criteria.", []string{"project_id"}, []string{"evidence"}),
		tool("report_status", "Record an evidence-backed coordination update; this does not mark project work accepted or completed.", []string{"project_id", "summary"}, []string{"evidence"}),
	}
}
func tool(name, description string, strings, arrays []string) Tool {
	props := map[string]any{}
	required := []string{}
	for _, k := range strings {
		props[k] = map[string]any{"type": "string"}
		required = append(required, k)
	}
	for _, k := range arrays {
		props[k] = map[string]any{"type": "array", "items": map[string]any{"type": "string"}}
		required = append(required, k)
	}
	if name == "delegate" {
		props["role"] = map[string]any{"type": "string", "enum": []string{"worker", "manager"}}
	}
	return Tool{Type: "function", Function: Function{Name: name, Description: description, Strict: true, Parameters: map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}}
}
func knownTool(name string) bool {
	for _, t := range Tools() {
		if t.Function.Name == name {
			return true
		}
	}
	return false
}
