package conversations

import "encoding/json"

// CallDetail is what a tool call acts on, shortened as a "Needs you"
// row shows it, or "" when the call or its input may hold a credential.
func CallDetail(tool string, input json.RawMessage) string {
	return approvalOf(toolCall{name: tool, input: callInput(tool, input)}).Detail
}
