package tests

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

type tenantArchitectureException struct {
	Reason string
	Risk   string
}

type tenantModelClassification struct {
	Registered bool
	Reason     string
}

var tenantModelClassifications = map[string]tenantModelClassification{
	"framework/audit/audit.go:Record":                                    {Reason: "mixed tenant/platform realm append-only audit fact"},
	"framework/idempotency/model.go:Record":                              {Registered: true},
	"framework/job/model.go:Run":                                         {Reason: "trusted worker must load the PostgreSQL fact before restoring AuthContext"},
	"framework/outbox/model.go:Event":                                    {Reason: "trusted dispatcher claims tenant and platform events before request AuthContext exists"},
	"modules/ai/agent_model.go:Agent":                                    {Registered: true},
	"modules/ai/agent_model.go:AgentToolBinding":                         {Registered: true},
	"modules/ai/agent_model.go:AgentVersion":                             {Registered: true},
	"modules/ai/agent_runtime_model.go:AgentRun":                         {Registered: true},
	"modules/ai/agent_runtime_model.go:AgentRunEvent":                    {Registered: true},
	"modules/ai/agent_runtime_model.go:AgentStep":                        {Registered: true},
	"modules/ai/agent_slot_model.go:AgentSlotBinding":                    {Registered: true},
	"modules/ai/agent_slot_model.go:AgentSlotPreference":                 {Registered: true},
	"modules/ai/conversation_model.go:AssistantStream":                   {Registered: true},
	"modules/ai/conversation_model.go:AssistantStreamDelta":              {Registered: true},
	"modules/ai/conversation_model.go:ContextSnapshot":                   {Registered: true},
	"modules/ai/conversation_model.go:Conversation":                      {Registered: true},
	"modules/ai/conversation_model.go:ConversationMessage":               {Registered: true},
	"modules/ai/knowledge_model.go:AgentKnowledgeBinding":                {Registered: true},
	"modules/ai/knowledge_model.go:KnowledgeBase":                        {Registered: true},
	"modules/ai/knowledge_model.go:KnowledgeChunk":                       {Registered: true},
	"modules/ai/knowledge_model.go:KnowledgeChunkPayload":                {Registered: true},
	"modules/ai/knowledge_model.go:KnowledgeDocument":                    {Registered: true},
	"modules/ai/knowledge_model.go:KnowledgeIndexVersion":                {Registered: true},
	"modules/ai/model.go:Alias":                                          {Registered: true},
	"modules/ai/model.go:Deployment":                                     {Registered: true},
	"modules/ai/model.go:ModelInvocation":                                {Registered: true},
	"modules/ai/model.go:Provider":                                       {Registered: true},
	"modules/ai/model.go:ToolCatalog":                                    {Registered: true},
	"modules/ai/model.go:ToolConfirmation":                               {Registered: true},
	"modules/ai/model.go:ToolInvocation":                                 {Registered: true},
	"modules/ai/model.go:ToolScope":                                      {Registered: true},
	"modules/ai/model.go:ToolSearchDocument":                             {Registered: true},
	"modules/ai/memory_model.go:MemoryItem":                              {Registered: true},
	"modules/ai/memory_model.go:MemoryItemVersion":                       {Registered: true},
	"modules/ai/memory_model.go:MemoryEvidence":                          {Registered: true},
	"modules/ai/memory_model.go:MemoryProjection":                        {Registered: true},
	"modules/ai/memory_model.go:MemoryDaySummary":                        {Registered: true},
	"modules/ai/memory_model.go:MemoryCurrentProfile":                    {Registered: true},
	"modules/ai/memory_model.go:MemoryEvaluation":                        {Registered: true},
	"modules/ai/memory_model.go:MemoryGovernanceDecision":                {Registered: true},
	"modules/ai/memory_model.go:MemoryRelation":                          {Registered: true},
	"modules/ai/memory_model.go:MemoryConsolidationBatch":                {Registered: true},
	"modules/demo/note_service.go:RevisionDoc":                           {Registered: true},
	"modules/demo/outbox_demo.go:Note":                                   {Registered: true},
	"modules/system/model/attachment.go:Attachment":                      {Registered: true},
	"modules/system/model/attachment.go:AttachmentLink":                  {Registered: true},
	"modules/system/model/models.go:AuthIdentity":                        {Reason: "private pre-auth store resolves identity before authenticated AuthContext exists"},
	"modules/system/model/models.go:AuthSession":                         {Reason: "private pre-auth store rotates sessions before request AuthContext exists"},
	"modules/system/model/models.go:Role":                                {Reason: "role authorization bootstrap uses fixed TenantRaw queries"},
	"modules/system/model/models.go:TenantSettingValue":                  {Registered: true},
	"modules/system/model/models.go:UserSettingValue":                    {Registered: true},
	"modules/system/model/models.go:User":                                {Reason: "private pre-auth store resolves users before authenticated AuthContext exists"},
	"modules/system/model/models.go:UserRole":                            {Reason: "role membership is constrained through users and roles by composite foreign keys"},
	"modules/system/notification_models.go:NotificationDelivery":         {Registered: true},
	"modules/system/notification_models.go:NotificationRequest":          {Registered: true},
	"modules/system/notification_models.go:NotificationTemplateOverride": {Registered: true},
	"modules/system/notification_models.go:UserNotification":             {Registered: true},
}

var tenantArchitectureExceptions = map[string]tenantArchitectureException{
	"framework/dblock/lock.go:LockForTenant:Exec":                       {Reason: "tenant advisory lock requires a fixed parameterized SQL expression", Risk: "caller must pass a scoped transaction and authenticated tenant"},
	"framework/dblock/lock.go:LockRowsForUpdate:Exec":                   {Reason: "multi-row lock requires a fixed parameterized SQL expression", Risk: "caller must pass a scoped transaction and authenticated tenant"},
	"framework/audit/audit.go:Write:Create":                             {Reason: "audit recorder writes tenant and platform realm facts from authenticated Action context", Risk: "mixed-realm append-only infrastructure"},
	"framework/idempotency/migrate.go:ensureConstraints:Exec":           {Reason: "schema migration maintains database constraints", Risk: "startup migration only"},
	"framework/job/runstore.go:Create:Create":                           {Reason: "Job runtime persists a validated envelope before asynchronous enqueue", Risk: "tenant and realm must come from ResolveTenantContext"},
	"framework/job/runstore.go:ListPendingEnqueue:Find":                 {Reason: "trusted enqueuer recovers pending runs across tenants", Risk: "private worker capability; tenant APIs use TenantRaw"},
	"framework/job/runstore.go:UpdateFields:Updates":                    {Reason: "trusted worker updates a previously loaded JobRun by immutable id", Risk: "must only follow GetTrusted or claimed run state"},
	"framework/job/migrate.go:ensureConstraints:Exec":                   {Reason: "schema migration maintains database constraints", Risk: "startup migration only"},
	"framework/outbox/delivery.go:ListByEvent:Find":                     {Reason: "dispatcher loads handler delivery facts for one trusted event", Risk: "private dispatcher capability"},
	"framework/outbox/delivery.go:RunHandler:Create":                    {Reason: "dispatcher creates an idempotent delivery fact", Risk: "private dispatcher capability"},
	"framework/outbox/delivery.go:RunHandler:First":                     {Reason: "dispatcher resolves one delivery fact by event and handler", Risk: "private dispatcher capability"},
	"framework/outbox/delivery.go:RunHandler:Save":                      {Reason: "dispatcher advances a claimed delivery state", Risk: "private dispatcher capability"},
	"framework/outbox/delivery.go:RunHandler:Updates":                   {Reason: "dispatcher records terminal delivery state", Risk: "private dispatcher capability"},
	"framework/outbox/dispatcher.go:markFailure:Updates":                {Reason: "dispatcher records publish failure for a claimed event", Risk: "private dispatcher capability"},
	"framework/outbox/dispatcher.go:processOne:Updates":                 {Reason: "dispatcher records publish success for a claimed event", Risk: "private dispatcher capability"},
	"framework/outbox/dispatcher.go:recoverStaleLocks:Updates":          {Reason: "dispatcher recovers expired claims across tenants", Risk: "private dispatcher capability"},
	"framework/outbox/migrate.go:ensureConstraints:Exec":                {Reason: "schema migration maintains database constraints", Risk: "startup migration only"},
	"framework/revision/revision.go:ConditionalUpdate:Table":            {Reason: "shared revision helper receives a repository-controlled table", Risk: "caller must pass a scoped transaction"},
	"framework/tenant/db.go:SystemExec:Exec":                            {Reason: "typed SystemDB parameterized SQL boundary", Risk: "platform system operator only"},
	"framework/tenant/db.go:SystemRaw:Raw":                              {Reason: "typed SystemDB parameterized SQL boundary", Risk: "platform system operator only"},
	"framework/tenant/db.go:TenantExec:Exec":                            {Reason: "typed TenantDB SQL boundary injects trusted tenant as first argument", Risk: "SQL must retain the tenant placeholder"},
	"framework/tenant/db.go:TenantRaw:Raw":                              {Reason: "typed TenantDB SQL boundary injects trusted tenant as first argument", Risk: "SQL must retain the tenant placeholder"},
	"modules/ai/invocation_service.go:RecoverStale:Raw":                 {Reason: "startup recovery discovers stale invocations before tenant context exists", Risk: "fixed private query; each row restores AuthContext before mutation"},
	"modules/ai/invocation_service.go:RecoverStale:Scan":                {Reason: "startup recovery scans a fixed private projection before tenant context exists", Risk: "each row restores AuthContext before mutation"},
	"modules/ai/knowledge_ingestion.go:Cleanup:Unscoped":                {Reason: "cleanup must inspect soft-deleted documents", Risk: "registered tenant callbacks still enforce authenticated tenant"},
	"modules/ai/knowledge_model.go:AutoMigrateKnowledge:Exec":           {Reason: "schema migration installs extension, generated column, and indexes", Risk: "startup migration only"},
	"modules/ai/knowledge_model.go:addKnowledgeConstraint:Exec":         {Reason: "schema migration installs named composite constraints", Risk: "startup migration only"},
	"modules/ai/model.go:AutoMigrate:Exec":                              {Reason: "schema migration upgrades credential storage", Risk: "startup migration only"},
	"modules/system/preauth_store.go:loadAuthorization:Raw":             {Reason: "authentication bootstrap query runs before full AuthContext exists", Risk: "private fixed SQL and explicit tenant parameter"},
	"modules/system/preauth_store.go:createSession:Create":              {Reason: "authentication bootstrap creates the session before request AuthContext exists", Risk: "private pre-auth store; tenant comes from verified identity"},
	"modules/system/preauth_store.go:findDefaultTenant:First":           {Reason: "login bootstrap resolves the only enabled tenant before identity verification", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:findIdentity:First":                {Reason: "login bootstrap resolves a verified identity before request AuthContext exists", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:findTenant:First":                  {Reason: "login bootstrap validates the tenant before identity verification", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:findUserByUsername:First":          {Reason: "password authentication loads a candidate user before request AuthContext exists", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:loadAuthorization:Count":           {Reason: "authorization bootstrap validates the session before request AuthContext exists", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:loadAuthorization:Scan":            {Reason: "authorization bootstrap loads role facts before request AuthContext exists", Risk: "private fixed SQL only"},
	"modules/system/preauth_store.go:upsertIdentity:Create":             {Reason: "authentication bootstrap reconciles a verified external identity", Risk: "private pre-auth store; tenant comes from verified identity"},
	"modules/system/seed.go:ensureActivationStatusConstraints:Exec":     {Reason: "schema migration maintains activation constraints", Risk: "startup migration only"},
	"modules/system/seed.go:ensureActivationStatusConstraints:Table":    {Reason: "schema migration validates a fixed internal table list", Risk: "startup migration only"},
	"modules/system/seed.go:ensureAttachmentStatusConstraint:Exec":      {Reason: "schema migration maintains attachment status constraint", Risk: "startup migration only"},
	"modules/system/seed.go:ensureAttachmentThumbStatusConstraint:Exec": {Reason: "schema migration maintains thumbnail status constraint", Risk: "startup migration only"},
	"modules/system/seed.go:ensureAuthVersionConstraint:Exec":           {Reason: "schema migration maintains authorization version constraint", Risk: "startup migration only"},
}

var manualTenantPredicateExceptions = map[string]tenantArchitectureException{
	"framework/job/runstore.go:CreateIdempotent:Where":         {Reason: "trusted enqueue path verifies tenant from AuthContext before persisting JobRun", Risk: "must remain private to the Job runtime"},
	"modules/system/preauth_store.go:findIdentity:Where":       {Reason: "identity lookup occurs before authenticated user context exists", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:findUserByID:Where":       {Reason: "session bootstrap validates the user before full AuthContext exists", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:findUserByUsername:Where": {Reason: "password authentication starts before full AuthContext exists", Risk: "private pre-auth store only"},
	"modules/system/preauth_store.go:loadAuthorization:Where":  {Reason: "authorization bootstrap validates the session before request AuthContext is established", Risk: "private pre-auth store only"},
}

func TestTenantDataAccessArchitecture(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	platformRoot := filepath.Clean(filepath.Join(filepath.Dir(current), ".."))
	fset := token.NewFileSet()
	var violations []string
	seenExceptions := map[string]bool{}
	discoveredModels := map[string]bool{}

	err := filepath.Walk(platformRoot, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			rel, _ := filepath.Rel(platformRoot, path)
			if rel == "tests" || (!strings.HasPrefix(rel, "framework") && !strings.HasPrefix(rel, "modules") && rel != ".") {
				if rel != "." {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, _ := filepath.Rel(platformRoot, path)
		rel = filepath.ToSlash(rel)
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		rawDBFields := collectRawDBFields(file)
		collectTenantModels(rel, file, discoveredModels)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			receiverName, receiverType := receiver(fn)
			inspectTenantFunction(fset, rel, fn.Name.Name, fn.Body, receiverName, rawDBFields[receiverType], seenExceptions, &violations)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for key, exception := range tenantArchitectureExceptions {
		if strings.TrimSpace(exception.Reason) == "" || strings.TrimSpace(exception.Risk) == "" {
			violations = append(violations, key+": exception requires reason and risk")
		}
		if !seenExceptions[key] {
			violations = append(violations, key+": stale architecture exception")
		}
	}
	for key, exception := range manualTenantPredicateExceptions {
		if strings.TrimSpace(exception.Reason) == "" || strings.TrimSpace(exception.Risk) == "" {
			violations = append(violations, key+": exception requires reason and risk")
		}
		if !seenExceptions[key] {
			violations = append(violations, key+": stale manual tenant predicate exception")
		}
	}
	for key := range discoveredModels {
		classification, ok := tenantModelClassifications[key]
		if !ok {
			violations = append(violations, key+": tenant-owned GORM model is not registered or classified")
			continue
		}
		if !classification.Registered && strings.TrimSpace(classification.Reason) == "" {
			violations = append(violations, key+": unregistered model classification requires a security reason")
		}
	}
	for key := range tenantModelClassifications {
		if !discoveredModels[key] {
			violations = append(violations, key+": stale tenant model classification")
		}
	}
	if len(violations) > 0 {
		t.Fatalf("tenant data architecture violations:\n%s", strings.Join(violations, "\n"))
	}
}

func TestTenantDataAccessDocumentationContract(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	workspaceRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	required := []string{
		"docs/development/tenant-data-access.md",
		".agent/rules/tenant-data-access.md",
	}
	for _, relative := range required {
		body, err := os.ReadFile(filepath.Join(workspaceRoot, relative))
		if err != nil {
			t.Fatalf("required tenant data access artifact %s: %v", relative, err)
		}
		if !strings.Contains(string(body), "TenantDB") || !strings.Contains(string(body), "SystemDB") {
			t.Fatalf("%s does not describe TenantDB/SystemDB", relative)
		}
	}
	mainRule, err := os.ReadFile(filepath.Join(workspaceRoot, ".agent/rules/main.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mainRule), "`tenant-data-access`") {
		t.Fatal("main Agent rule does not index tenant-data-access")
	}

	for _, root := range []string{"docs/development", ".agent/rules", ".agent/templates"} {
		err = filepath.Walk(filepath.Join(workspaceRoot, root), func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() || (!strings.HasSuffix(path, ".md") && !strings.HasSuffix(path, ".tpl")) {
				return nil
			}
			base := filepath.Base(path)
			if base == "tenant-data-access.md" || base == "tenant-data-access-inventory.md" {
				return nil
			}
			body, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			text := string(body)
			for _, forbidden := range []string{"tenant.ScopeByTenant", "tenant.SystemScope", "tenant.RuntimeDB", "db.Model("} {
				if strings.Contains(text, forbidden) {
					relative, _ := filepath.Rel(workspaceRoot, path)
					t.Errorf("%s recommends removed or unsafe database pattern %q", filepath.ToSlash(relative), forbidden)
				}
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func collectTenantModels(filePath string, file *ast.File, out map[string]bool) {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				if len(field.Names) != 1 || field.Names[0].Name != "TenantID" || field.Tag == nil {
					continue
				}
				tag, err := strconv.Unquote(field.Tag.Value)
				if err == nil && strings.Contains(tag, `gorm:"`) {
					out[filePath+":"+typeSpec.Name.Name] = true
				}
			}
		}
	}
}

func inspectTenantFunction(fset *token.FileSet, file, function string, body *ast.BlockStmt, receiverName string, rawDBFields map[string]bool, seen map[string]bool, violations *[]string) {
	ast.Inspect(body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && (ident.Name == "SystemScope" || ident.Name == "RuntimeDB") {
			*violations = append(*violations, fmt.Sprintf("%s:%d: removed compatibility entrypoint %s", file, fset.Position(call.Pos()).Line, ident.Name))
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		method := selector.Sel.Name
		key := file + ":" + function + ":" + method

		if isGormTerminal(method) {
			root, field := rootField(selector.X)
			if root == receiverName && rawDBFields[field] {
				if _, allowed := tenantArchitectureExceptions[key]; allowed {
					seen[key] = true
				} else {
					*violations = append(*violations, fmt.Sprintf("%s:%d: tenant business operation %s starts from raw *gorm.DB field %s.%s", file, fset.Position(call.Pos()).Line, method, root, field))
				}
			}
		}

		switch method {
		case "Raw", "Exec", "Table", "Joins", "Unscoped":
			if _, allowed := tenantArchitectureExceptions[key]; allowed {
				seen[key] = true
			} else {
				*violations = append(*violations, fmt.Sprintf("%s:%d: unregistered %s call (%s)", file, fset.Position(call.Pos()).Line, method, key))
			}
		}

		if method == "Where" || method == "Or" || method == "Not" || method == "Having" {
			sql, literal := firstStringArgument(call)
			if literal {
				if strings.Contains(strings.ToLower(sql), "tenant_id") {
					if _, allowed := manualTenantPredicateExceptions[key]; allowed {
						seen[key] = true
					} else {
						*violations = append(*violations, fmt.Sprintf("%s:%d: manual tenant predicate (%s)", file, fset.Position(call.Pos()).Line, key))
					}
				}
				if call.Ellipsis == token.NoPos {
					want := strings.Count(sql, "?")
					got := len(call.Args) - 1
					if want != got {
						*violations = append(*violations, fmt.Sprintf("%s:%d: %s placeholder mismatch: want %d arguments, got %d", file, fset.Position(call.Pos()).Line, method, want, got))
					}
				}
			}
		}
		return true
	})
}

func collectRawDBFields(file *ast.File) map[string]map[string]bool {
	result := map[string]map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			typeSpec, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structType, ok := typeSpec.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range structType.Fields.List {
				if !isGormDBPointer(field.Type) {
					continue
				}
				if result[typeSpec.Name.Name] == nil {
					result[typeSpec.Name.Name] = map[string]bool{}
				}
				for _, name := range field.Names {
					result[typeSpec.Name.Name][name.Name] = true
				}
			}
		}
	}
	return result
}

func isGormDBPointer(expr ast.Expr) bool {
	star, ok := expr.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "DB" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "gorm"
}

func receiver(fn *ast.FuncDecl) (string, string) {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return "", ""
	}
	name := fn.Recv.List[0].Names[0].Name
	expr := fn.Recv.List[0].Type
	if pointer, ok := expr.(*ast.StarExpr); ok {
		expr = pointer.X
	}
	ident, _ := expr.(*ast.Ident)
	if ident == nil {
		return name, ""
	}
	return name, ident.Name
}

func rootField(expr ast.Expr) (string, string) {
	switch value := expr.(type) {
	case *ast.CallExpr:
		if selector, ok := value.Fun.(*ast.SelectorExpr); ok {
			return rootField(selector.X)
		}
	case *ast.SelectorExpr:
		if base, ok := value.X.(*ast.Ident); ok {
			return base.Name, value.Sel.Name
		}
		return rootField(value.X)
	}
	return "", ""
}

func isGormTerminal(method string) bool {
	switch method {
	case "First", "Find", "Take", "Last", "Create", "Save", "Updates", "Update", "Delete", "Count", "Pluck", "Scan":
		return true
	default:
		return false
	}
}

func firstStringArgument(call *ast.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	literal, ok := call.Args[0].(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	value, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return value, true
}
