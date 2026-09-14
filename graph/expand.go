package graph

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// expandShorthand lowers convenience links into the existing outbound model.
// Only slices and changed settings are copied; stored nodes/settings stay untouched.
func expandShorthand(st State) State {
	out := State{Nodes: append([]Node(nil), st.Nodes...), Edges: append([]Edge(nil), st.Edges...)}
	byID := make(map[string]Node, len(st.Nodes))
	indexByID := make(map[string]int, len(st.Nodes))
	used := map[string]bool{}
	for i, n := range st.Nodes {
		byID[n.ID] = n
		indexByID[n.ID] = i
		used[n.ID], used[strings.TrimSpace(n.Tag)] = true, true
	}
	for _, e := range st.Edges {
		used[e.ID] = true
	}
	// Stable allocation even if persisted node/edge order changes.
	sort.Slice(out.Edges, func(i, j int) bool {
		if out.Edges[i].SourceID != out.Edges[j].SourceID {
			return out.Edges[i].SourceID < out.Edges[j].SourceID
		}
		return out.Edges[i].TargetID < out.Edges[j].TargetID
	})
	for i := range out.Edges {
		e := &out.Edges[i]
		src, dst := byID[e.SourceID], byID[e.TargetID]
		if dst.Kind != KindInbound || (src.Kind != KindInbound && src.Kind != KindBalancer && src.Kind != KindRule) {
			continue
		}
		tag := uniqueGraphName("cascadia-relay-"+src.ID+"-"+dst.ID, used)
		out.Nodes = append(out.Nodes, Node{ID: tag, Tag: tag, NodeID: src.NodeID,
			Kind: KindOutbound, Protocol: dst.Protocol, PosY: dst.PosY})
		e.TargetID = tag
		// Appending can reallocate Edges, so don't use e afterwards.
		out.Edges = append(out.Edges, Edge{ID: uniqueGraphName(tag+"-edge", used), SourceID: tag, TargetID: dst.ID})
		if src.Kind == KindBalancer {
			n := &out.Nodes[indexByID[src.ID]]
			s, _ := ParseBalancerSettings(n.Settings) // already validated
			if s.Default == dst.Tag {
				s.Default = tag
				n.Settings, _ = json.Marshal(s)
			}
		}
	}
	return out
}

func uniqueGraphName(base string, used map[string]bool) string {
	name := base
	for suffix := 1; used[name]; suffix++ {
		name = fmt.Sprintf("%s-%d", base, suffix)
	}
	used[name] = true
	return name
}
