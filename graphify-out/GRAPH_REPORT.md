# Graph Report - .  (2026-09-18)

## Corpus Check
- Corpus is ~43,166 words - fits in a single context window. You may not need a graph.

## Summary
- 606 nodes · 1141 edges · 29 communities (27 shown, 2 thin omitted)
- Extraction: 88% EXTRACTED · 12% INFERRED · 0% AMBIGUOUS · INFERRED: 138 edges (avg confidence: 0.8)
- Token cost: 0 input · 0 output

## Community Hubs (Navigation)
- [[_COMMUNITY_Protocol bindings|Protocol bindings]]
- [[_COMMUNITY_Authentication API|Authentication API]]
- [[_COMMUNITY_Graph persistence|Graph persistence]]
- [[_COMMUNITY_Graph generation tests|Graph generation tests]]
- [[_COMMUNITY_Graph generation|Graph generation]]
- [[_COMMUNITY_Graph API and deployment|Graph API and deployment]]
- [[_COMMUNITY_Web API client|Web API client]]
- [[_COMMUNITY_Node gRPC client|Node gRPC client]]
- [[_COMMUNITY_gRPC service bindings|gRPC service bindings]]
- [[_COMMUNITY_Svelte graph interface|Svelte graph interface]]
- [[_COMMUNITY_VPN user API|VPN user API]]
- [[_COMMUNITY_Subscription link generation|Subscription link generation]]
- [[_COMMUNITY_Users page and dependencies|Users page and dependencies]]
- [[_COMMUNITY_Graph validation|Graph validation]]
- [[_COMMUNITY_Authentication tests|Authentication tests]]
- [[_COMMUNITY_Subscription route|Subscription route]]
- [[_COMMUNITY_TypeScript configuration|TypeScript configuration]]
- [[_COMMUNITY_Graph storage tests|Graph storage tests]]
- [[_COMMUNITY_Project documentation|Project documentation]]
- [[_COMMUNITY_Browser graph tests|Browser graph tests]]
- [[_COMMUNITY_Release workflow|Release workflow]]
- [[_COMMUNITY_Container deployment|Container deployment]]
- [[_COMMUNITY_XYFlow integration|XYFlow integration]]

## God Nodes (most connected - your core abstractions)
1. `mustJSON()` - 26 edges
2. `writeJSON()` - 21 edges
3. `Generate()` - 20 edges
4. `twoNodeCascade()` - 18 edges
5. `generateNodeConfig()` - 16 edges
6. `ParseInboundSettings()` - 15 edges
7. `GetStatusResponse` - 15 edges
8. `T` - 14 edges
9. `newTestServer()` - 13 edges
10. `Store` - 13 edges

## Surprising Connections (you probably didn't know these)
- `Production Frontend Entrypoint` --semantically_similar_to--> `Development Frontend Entrypoint`  [INFERRED] [semantically similar]
  api/static/index.html → web/index.html
- `newTestServer()` --calls--> `NewAuthenticator()`  [INFERRED]
  api/auth_test.go → auth/auth.go
- `newTestServer()` --calls--> `New()`  [INFERRED]
  api/auth_test.go → db/store.go
- `TestSeedAdminGenerated()` --calls--> `New()`  [INFERRED]
  api/auth_test.go → db/store.go
- `TestSaveGraphValidatesGeneratedRelayUser()` --calls--> `ParseInboundSettings()`  [INFERRED]
  api/graph_handlers_test.go → graph/model.go

## Import Cycles
- None detected.

## Hyperedges (group relationships)
- **Cascade Configuration Flow** — readme_cascade_node_management, readme_cascade_graph_editor, readme_sing_box_configuration, readme_grpc_configuration_deployment [EXTRACTED 1.00]

## Communities (29 total, 2 thin omitted)

### Community 0 - "Protocol bindings"
Cohesion: 0.06
Nodes (13): Message, MessageState, GetConfigRequest, GetConfigResponse, GetStatusRequest, GetStatusResponse, file_proto_node_node_proto_init(), file_proto_node_node_proto_rawDescGZIP() (+5 more)

### Community 1 - "Authentication API"
Cohesion: 0.07
Nodes (39): decodeJSON(), Authenticator, Duration, Request, ResponseWriter, Store, authHandlers, ClearSessionCookie() (+31 more)

### Community 2 - "Graph persistence"
Cohesion: 0.06
Nodes (18): DB, Graph, btoi(), Store, State, Node, Store, scanPanelUser() (+10 more)

### Community 3 - "Graph generation tests"
Cohesion: 0.13
Nodes (46): Generate(), T, mustRealityPair(), TestGenerateHysteria2AndTUIC(), TestGenerateRealityMirror(), TestGenerateRejectsInvalid(), TestGenerateRulesFromEdges(), TestGenerateTwoNodeCascade() (+38 more)

### Community 4 - "Graph generation"
Cohesion: 0.08
Nodes (40): BalancerSettings, Edge, expandShorthand(), State, uniqueGraphName(), applyRuleMatch(), clientTLSFromInbound(), generateNodeConfig() (+32 more)

### Community 5 - "Graph API and deployment"
Cohesion: 0.12
Nodes (21): deployReport, errBad(), Handler, PhysNode, Request, ResponseWriter, State, writeErr422() (+13 more)

### Community 6 - "Web API client"
Cohesion: 0.07
Nodes (32): api(), ApiError, authEvents, changePassword(), createUser(), deleteUser(), DeployReport, DeployResult (+24 more)

### Community 7 - "Node gRPC client"
Cohesion: 0.10
Nodes (24): Certificate, ClientConn, Client, Context, Status, UpdateConfigResponse, NewClient(), Context (+16 more)

### Community 8 - "gRPC service bindings"
Cohesion: 0.14
Nodes (20): ClientConnInterface, New(), GetConfigRequest, GetConfigResponse, NewNodeServiceClient(), _NodeService_GetConfig_Handler(), _NodeService_GetStatus_Handler(), _NodeService_UpdateConfig_Handler() (+12 more)

### Community 9 - "Svelte graph interface"
Cohesion: 0.09
Nodes (16): $lib/api, ./GraphDock.svelte, ./GraphEditor.svelte, warnCount, { onclose }, $lib/element, $components/GraphsPage.svelte, $components/Login.svelte (+8 more)

### Community 10 - "VPN user API"
Cohesion: 0.17
Nodes (18): deployOut, Handler, PanelCreds, Request, ResponseWriter, panelCreds(), userBody, userOpResponse (+10 more)

### Community 11 - "Subscription link generation"
Cohesion: 0.17
Nodes (23): applyClientTLS(), applyTransport(), applyVMessTLS(), buildShareLink(), InboundSettings, Node, PanelCreds, PhysNode (+15 more)

### Community 12 - "Users page and dependencies"
Cohesion: 0.09
Nodes (19): dependencies, qrcode, svelte, @xyflow/svelte, devDependencies, @playwright/test, @sveltejs/vite-plugin-svelte, @types/qrcode (+11 more)

### Community 13 - "Graph validation"
Cohesion: 0.26
Nodes (10): Issue, Node, PhysNode, RuleSettings, State, kindLabel(), ruleHasMatch(), sortIssues() (+2 more)

### Community 14 - "Authentication tests"
Cohesion: 0.31
Nodes (13): do(), Handler, T, newTestServer(), TestAnonymousRejected(), TestBearerTokenWorks(), TestChangePassword(), TestLoginFlow() (+5 more)

### Community 15 - "Subscription route"
Cohesion: 0.15
Nodes (12): Authenticator, Duration, Handler, NewRouter(), Handler, Request, ResponseWriter, Config (+4 more)

### Community 16 - "TypeScript configuration"
Cohesion: 0.13
Nodes (14): compilerOptions, esModuleInterop, isolatedModules, jsx, lib, module, moduleResolution, noEmit (+6 more)

### Community 17 - "Graph storage tests"
Cohesion: 0.38
Nodes (12): InboundUser, Node, Store, T, mustLoadRelayUser(), newTestStore(), seedNode(), TestDuplicateGraphNameFails() (+4 more)

### Community 18 - "Project documentation"
Cohesion: 0.20
Nodes (10): Authentication, Cascade Graph Editor, Cascade Node Management, Cascadia Panel, gRPC Configuration Deployment, sing-box Configuration, Static Go Binary, Svelte Application Shell (+2 more)

### Community 20 - "Release workflow"
Cohesion: 0.50
Nodes (4): Release Versioning, Docker Image Build, GitHub Container Registry, Multi-Architecture Images

### Community 21 - "Container deployment"
Cohesion: 0.67
Nodes (3): Panel Service, Persistent Panel Data, Panel Resource Limits

## Knowledge Gaps
- **124 isolated node(s):** `Store`, `Authenticator`, `LoginLimiter`, `Duration`, `ResponseRecorder` (+119 more)
  These have ≤1 connection - possible missing edges or undocumented components.
- **2 thin communities (<3 nodes) omitted from report** — run `graphify query` to explore isolated nodes.

## Suggested Questions
_Questions this graph is uniquely positioned to answer:_

- **Why does `ParseInboundSettings()` connect `Graph generation` to `Graph persistence`, `VPN user API`, `Subscription link generation`, `Graph validation`, `Authentication tests`, `Graph storage tests`?**
  _High betweenness centrality (0.184) - this node is a cross-community bridge._
- **Why does `New()` connect `gRPC service bindings` to `Graph persistence`, `Node gRPC client`, `Authentication tests`, `Subscription route`, `Graph storage tests`?**
  _High betweenness centrality (0.106) - this node is a cross-community bridge._
- **Why does `writeJSON()` connect `Graph API and deployment` to `Authentication API`, `VPN user API`?**
  _High betweenness centrality (0.091) - this node is a cross-community bridge._
- **Are the 16 inferred relationships involving `mustJSON()` (e.g. with `TestGenerateHysteria2AndTUIC()` and `TestGenerateRealityMirror()`) actually correct?**
  _`mustJSON()` has 16 INFERRED edges - model-reasoned connections that need verification._
- **Are the 13 inferred relationships involving `writeJSON()` (e.g. with `.Login()` and `.Me()`) actually correct?**
  _`writeJSON()` has 13 INFERRED edges - model-reasoned connections that need verification._
- **Are the 16 inferred relationships involving `Generate()` (e.g. with `.DeployGraph()` and `.GraphConfigs()`) actually correct?**
  _`Generate()` has 16 INFERRED edges - model-reasoned connections that need verification._
- **Are the 3 inferred relationships involving `twoNodeCascade()` (e.g. with `TestGenerateTwoNodeCascade()` and `TestRelayUserNoClients()`) actually correct?**
  _`twoNodeCascade()` has 3 INFERRED edges - model-reasoned connections that need verification._