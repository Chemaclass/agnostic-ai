package authority

import "slices"

// notModeledRoutes are the routes the view never evaluates.
var notModeledRoutes = []string{
	"delegation to other agents",
	"other MCP servers reaching the same system",
	"web access",
	"anything a running tool does that configuration does not describe",
}

// Route names.
const (
	routeShellAny   = "shell: any command"
	routeLocalFiles = "local files"
	routeFilesRead  = "local files: read"
	routeFilesEdit  = "local files: edit"
)

// route is one checked route with the sentence a client shows when it
// is open.
type route struct {
	status   RouteStatus
	sentence string
}

// open reports a route the subject can take: permitted, permitted with
// approval, or left to the tool's default.
func (r route) open() bool {
	_, ok := routePhrases[r.status.Effect]
	return ok
}

// applyRoutes fills each operation's route coverage and alternate
// routes from the subject's effective decisions on the shell and local
// files resources. An empty AlternateRoutes then means only that no
// checked route is open.
func applyRoutes(resources []*builtResource) {
	byID := map[string]Operation{}
	var shells []*builtResource
	for _, br := range resources {
		for _, op := range br.res.Operations {
			byID[op.ID] = op
		}
		if br.info.kind == kindShell && br.info.shell != nil {
			shells = append(shells, br)
		}
	}
	effectOf := func(id string) Effect {
		if op, ok := byID[id]; ok {
			return op.Effect
		}
		return EffectUnresolved
	}
	shellAny := route{status: RouteStatus{Route: routeShellAny, Effect: effectOf(operationID("shell-any", "run"))}}
	shellAny.sentence = "Shell access is " + routePhrases[shellAny.status.Effect] + " for this subject and could reach the same system through its command-line tools; this rule does not govern that route."
	shellRoutes := []route{shellAny}
	for _, br := range shells {
		r := route{status: RouteStatus{Route: "shell: " + br.res.Label, Effect: effectOf(operationID(br.res.ID, "run"))}}
		r.sentence = "Shell commands matching " + br.res.Label + " are " + routePhrases[r.status.Effect] + " for this subject and could reach the same system; this rule does not govern that route."
		shellRoutes = append(shellRoutes, r)
	}
	read := effectOf(operationID("files-all", capRead))
	edit := effectOf(operationID("files-all", capEdit))
	filesForMCP := route{status: RouteStatus{Route: routeLocalFiles, Effect: read, Note: "read on All project files"}}
	filesForMCP.sentence = "Reading local files is " + routePhrases[read] + " for this subject and could reach the same system's data on disk, such as its configuration or credentials; this rule does not govern that route."
	filesForShell := []route{
		{status: RouteStatus{Route: routeFilesRead, Effect: read}, sentence: "Reading local files is " + routePhrases[read] + " for this subject; this rule does not govern that route."},
		{status: RouteStatus{Route: routeFilesEdit, Effect: edit}, sentence: "Editing local files is " + routePhrases[edit] + " for this subject and could change what commands do; this rule does not govern that route."},
	}
	for _, br := range resources {
		var checked []route
		switch br.info.kind {
		case kindShell:
			checked = filesForShell
		case kindMCP:
			checked = append(slices.Clone(shellRoutes), filesForMCP)
		default:
			checked = shellRoutes
		}
		for i := range br.res.Operations {
			op := &br.res.Operations[i]
			op.RouteCoverage = RouteCoverage{Checked: make([]RouteStatus, 0, len(checked)), NotModeled: slices.Clone(notModeledRoutes)}
			op.AlternateRoutes = []string{}
			for _, r := range checked {
				op.RouteCoverage.Checked = append(op.RouteCoverage.Checked, r.status)
				if r.open() {
					op.AlternateRoutes = append(op.AlternateRoutes, r.sentence)
				}
			}
		}
	}
}
