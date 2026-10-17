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

// routeVerb words a route's decision inside a sentence with the same
// qualification the route's own badge carries: "is permitted" when the
// generated file is in sync, "would be permitted once synced" while it
// is stale, missing, or unchecked. singular picks "is" over "are".
func routeVerb(singular bool, st RouteStatus) string {
	phrase := routePhrases[st.Effect]
	if st.Hypothetical {
		return "would be " + phrase + " once synced"
	}
	if singular {
		return "is " + phrase
	}
	return "are " + phrase
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
	// statusOf takes the route's decision, installed state, and
	// hypothetical flag from the operation the route stands for, so the
	// sentence and the badge a client shows agree with that operation.
	statusOf := func(name, id string) RouteStatus {
		st := RouteStatus{Route: name, Effect: EffectUnresolved}
		if op, ok := byID[id]; ok {
			st.Effect, st.Hypothetical, st.Installed = op.Effect, op.Hypothetical, op.Installed
		}
		return st
	}
	shellAny := route{status: statusOf(routeShellAny, operationID("shell-any", "run"))}
	shellAny.sentence = "Shell access " + routeVerb(true, shellAny.status) + " for this subject and could reach the same system through its command-line tools; this rule does not govern that route."
	shellRoutes := []route{shellAny}
	for _, br := range shells {
		r := route{status: statusOf("shell: "+br.res.Label, operationID(br.res.ID, "run"))}
		r.sentence = "Shell commands matching " + br.res.Label + " " + routeVerb(false, r.status) + " for this subject and could reach the same system; this rule does not govern that route."
		shellRoutes = append(shellRoutes, r)
	}
	read := statusOf(routeLocalFiles, operationID("files-all", capRead))
	read.Note = "read on All project files"
	edit := statusOf(routeFilesEdit, operationID("files-all", capEdit))
	filesForMCP := route{status: read}
	filesForMCP.sentence = "Reading local files " + routeVerb(true, read) + " for this subject and could reach the same system's data on disk, such as its configuration or credentials; this rule does not govern that route."
	readForShell := read
	readForShell.Route, readForShell.Note = routeFilesRead, ""
	filesForShell := []route{
		{status: readForShell, sentence: "Reading local files " + routeVerb(true, readForShell) + " for this subject; this rule does not govern that route."},
		{status: edit, sentence: "Editing local files " + routeVerb(true, edit) + " for this subject and could change what commands do; this rule does not govern that route."},
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
