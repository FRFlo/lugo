package lsp

import "github.com/FRFlo/lugo/ast"

// fiveMASTFacts records the parent-derived facts used by the FiveM audits.
// Each audit builds this per-document index instead of repeatedly walking the
// complete tree or each node's ancestors.
type fiveMASTFacts struct {
	callNames             []string
	enclosingCall         []ast.NodeID
	nearestFunction       []ast.NodeID
	identifiersByFunction map[ast.NodeID][]ast.NodeID
	sourceIdentifiers     []ast.NodeID
	yieldCallsByFunction  map[ast.NodeID][]fiveMASTCallFact
	loops                 map[ast.NodeID]fiveMLoopFact
}

type fiveMASTCallFact struct {
	id         ast.NodeID
	start, end uint32
}

type fiveMLoopFact struct {
	waitZero bool
	yield    bool
	lookup   bool
}

func newFiveMASTFacts(doc *Document) *fiveMASTFacts {
	facts := &fiveMASTFacts{
		identifiersByFunction: make(map[ast.NodeID][]ast.NodeID),
		yieldCallsByFunction:  make(map[ast.NodeID][]fiveMASTCallFact),
		loops:                 make(map[ast.NodeID]fiveMLoopFact),
	}
	if doc == nil || doc.Tree == nil {
		return facts
	}

	tree := doc.Tree
	n := len(tree.Nodes)
	facts.callNames = make([]string, n)
	facts.enclosingCall = make([]ast.NodeID, n)
	facts.nearestFunction = make([]ast.NodeID, n)
	parent := make([]ast.NodeID, n)
	firstChild := make([]ast.NodeID, n)
	nextSibling := make([]ast.NodeID, n)
	for i := range parent {
		parent[i] = ast.InvalidNode
		firstChild[i] = ast.InvalidNode
		nextSibling[i] = ast.InvalidNode
		facts.enclosingCall[i] = ast.InvalidNode
		facts.nearestFunction[i] = ast.InvalidNode
	}
	for id := ast.NodeID(1); int(id) < n; id++ {
		p := tree.Nodes[id].Parent
		if p == ast.InvalidNode || int(p) >= n || p == id {
			continue
		}
		parent[id] = p
		nextSibling[id] = firstChild[p]
		firstChild[p] = id
	}

	type frame struct {
		id, child ast.NodeID
		function  ast.NodeID
		call      ast.NodeID
	}
	postorder := make([]ast.NodeID, 0, n)
	visited := make([]bool, n)
	for root := ast.NodeID(0); int(root) < n; root++ {
		if parent[root] != ast.InvalidNode || visited[root] {
			continue
		}
		stack := []frame{{id: root, child: firstChild[root], function: ast.InvalidNode, call: ast.InvalidNode}}
		for len(stack) > 0 {
			top := &stack[len(stack)-1]
			if !visited[top.id] {
				visited[top.id] = true
				node := tree.Nodes[top.id]
				if node.Kind == ast.KindFunctionExpr || node.Kind == ast.KindFunctionStmt {
					top.function = top.id
				}
				if node.Kind == ast.KindCallExpr || node.Kind == ast.KindMethodCall {
					top.call = top.id
				}
				facts.nearestFunction[top.id] = top.function
				facts.enclosingCall[top.id] = top.call
			}
			if top.child != ast.InvalidNode {
				child := top.child
				top.child = nextSibling[child]
				stack = append(stack, frame{id: child, child: firstChild[child], function: top.function, call: top.call})
				continue
			}
			postorder = append(postorder, top.id)
			stack = stack[:len(stack)-1]
		}
	}

	loopState := make([]fiveMLoopFact, n)
	for _, id := range postorder {
		node := tree.Nodes[id]
		if node.Kind == ast.KindCallExpr || node.Kind == ast.KindMethodCall {
			name := performanceNodeCallName(doc, id)
			facts.callNames[id] = name
			if isFiveMYieldCall(doc, id) {
				facts.yieldCallsByFunction[facts.nearestFunction[id]] = append(facts.yieldCallsByFunction[facts.nearestFunction[id]], fiveMASTCallFact{id: id, start: node.Start, end: node.End})
			}
			if name == "Wait" || len(name) > len(".Wait") && name[len(name)-len(".Wait"):] == ".Wait" {
				arg := callArgument(doc, node, 0)
				if arg != ast.InvalidNode && tree.Nodes[arg].Kind == ast.KindNumber && string(doc.Source()[tree.Nodes[arg].Start:tree.Nodes[arg].End]) == "0" {
					loopState[id].waitZero = true
				}
				loopState[id].yield = true
			}
			if name == "coroutine.yield" || len(name) > len(".yield") && name[len(name)-len(".yield"):] == ".yield" {
				loopState[id].yield = true
			}
			if performanceLookupCall(name) {
				loopState[id].lookup = true
			}
		}
		if node.Kind == ast.KindIdent {
			facts.identifiersByFunction[facts.nearestFunction[id]] = append(facts.identifiersByFunction[facts.nearestFunction[id]], id)
			if node.Start < node.End && node.End <= uint32(len(doc.Source())) && string(doc.Source()[node.Start:node.End]) == "source" {
				facts.sourceIdentifiers = append(facts.sourceIdentifiers, id)
			}
		}
		if parent[id] != ast.InvalidNode {
			loopState[parent[id]].waitZero = loopState[parent[id]].waitZero || loopState[id].waitZero
			loopState[parent[id]].yield = loopState[parent[id]].yield || loopState[id].yield
			loopState[parent[id]].lookup = loopState[parent[id]].lookup || loopState[id].lookup
		}
	}
	for id := ast.NodeID(1); int(id) < n; id++ {
		switch tree.Nodes[id].Kind {
		case ast.KindWhile, ast.KindRepeat, ast.KindForNum, ast.KindForIn:
			facts.loops[id] = loopState[id]
		}
	}
	return facts
}
