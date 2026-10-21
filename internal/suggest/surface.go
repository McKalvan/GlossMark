// @gloss implements gmi:MarkerSuggester scope=file

package suggest

import (
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/mckalvan/glossmark/internal/marker"
)

// Roles a declaration plays. The role goes into the calibration basis,
// because the same match is worth more on a table than on a helper.
const (
	roleType     = "type"     // class, interface, struct, enum, proto message
	roleFunc     = "func"     // top-level function, Go method
	roleMember   = "member"   // method inside a class
	roleTest     = "test"     // test function or class in a test path, Gherkin
	roleTable    = "table"    // SQL table, view, function, procedure; Prisma model
	roleResource = "resource" // Terraform resource or module
	roleHeading  = "heading"  // Markdown heading
	roleWorkload = "workload" // Kubernetes Deployment, CronJob, ...
	rolePage     = "page"     // a page component (presents)
	roleShell    = "shell"    // a layout or app shell (hosts)
)

// surface decides whether a declaration is a public surface a marker
// belongs on (MARKING.md: the handler, the model, the table, the test), and
// what role it plays. It skips what the first adoptions deleted by the
// dozen (#68): package clauses and namespace modules, Rust impl blocks, SQL
// indexes and types, module-level variables, language-private declarations,
// every declaration in a test file that is not a test (fakes, stubs,
// fixtures, builders), and the Terraform resources that only support
// another (ADR 0032).
func surface(d marker.Decl, lines []string, path string, test bool) (role string, ok bool) {
	ext := strings.ToLower(filepath.Ext(path))
	sig := strings.TrimSpace(lines[d.Line-1])
	name := lastPart(d.Symbol)
	if strings.HasPrefix(name, "_") || name == "main" || name == "init" || name == "" {
		return "", false
	}
	switch {
	case d.Kind == "package", d.Kind == "key", d.Kind == "item", d.Kind == "alter", d.Kind == "block", d.Kind == "instruction":
		return "", false
	case ext == ".rb" && strings.HasPrefix(sig, "module "):
		return "", false
	case ext == ".rs" && rustImpl.MatchString(sig):
		return "", false
	}
	if test || strings.HasSuffix(path, ".feature") {
		if !isTest(d, sig, path) {
			return "", false
		}
		return roleTest, true
	}
	if private(d, sig, ext) {
		return "", false
	}
	switch d.Kind {
	case "sql":
		kind := strings.SplitN(d.Symbol, ".", 2)[0]
		switch kind {
		case "table", "view", "materialized view":
			return roleTable, true
		case "function", "procedure":
			// A routine does work: it implements a process, as the
			// function that calls it would (ADR 0032).
			return roleFunc, true
		}
		return "", false
	case "model":
		// A Prisma model or view is a table.
		return roleTable, true
	case "proto":
		if strings.HasPrefix(sig, "rpc") {
			return roleFunc, true
		}
		return roleType, true
	case "resource":
		if strings.HasPrefix(d.Symbol, "data.") || supporting(d.Symbol) {
			return "", false
		}
		return roleResource, true
	case "heading":
		return roleHeading, true
	case "variable":
		// Module-level variables are configuration, loggers and DI wiring
		// (router = APIRouter(), log = get_logger()); an exported function
		// value or component (export const BookingPage = () => ...) is the
		// exception.
		if (ext == ".ts" || ext == ".tsx" || ext == ".js" || ext == ".jsx" || ext == ".mjs") && strings.HasPrefix(sig, "export") &&
			(strings.Contains(sig, "=>") || strings.Contains(sig, "function")) {
			return pageRole(path, roleFunc), true
		}
		return "", false
	}
	if typeLike(d, sig) {
		return roleType, true
	}
	if d.Parent != "" {
		return roleMember, true
	}
	return pageRole(path, roleFunc), true
}

// supportingResource matches the Terraform resource types that configure,
// secure or wire up another resource: a subnet group or a role is not the
// database or the worker it is named after (aws_db_subnet_group.booking,
// aws_iam_role.reminders, #65). The resource it supports carries the
// marker.
var supportingResource = regexp.MustCompile(`_(subnet_group|parameter_group|option_group|iam_role|policy|attachment|event_target|lifecycle_configuration|versioning|public_access_block|encryption_configuration|kms_key|kms_alias|security_group|security_group_rule|task_definition|log_group|metric_alarm|permission)$`)

// supporting reports a Terraform resource that only supports another.
func supporting(sym string) bool {
	parts := strings.Split(sym, ".")
	return len(parts) == 3 && parts[0] == "resource" && supportingResource.MatchString(parts[1])
}

// pageRole turns a component in a page file into a page, and one in a
// layout or app shell into a shell.
func pageRole(path, role string) string {
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	ext := strings.ToLower(filepath.Ext(path))
	if ext != ".tsx" && ext != ".jsx" && ext != ".vue" && ext != ".svelte" {
		return role
	}
	switch {
	case base == "layout" || base == "_app" || base == "App" || base == "_document":
		return roleShell
	case base == "page" || strings.Contains(filepath.ToSlash(path), "/pages/") && !strings.Contains(filepath.ToSlash(path), "/pages/api/"):
		return rolePage
	}
	return role
}

var (
	rustImpl = regexp.MustCompile(`^\s*(?:unsafe\s+)?impl\b`)
	typeKw   = regexp.MustCompile(`\b(class|interface|enum|record|struct|trait|object|union|message|type)\s+[A-Za-z_]`)
	fnKw     = regexp.MustCompile(`(^|\s)(fn|def|func|function)\s`)
)

// typeLike reports a declaration that defines a data shape rather than
// behaviour.
func typeLike(d marker.Decl, sig string) bool {
	switch d.Kind {
	case "class", "interface", "enum", "struct":
		return true
	case "function", "method":
		return false
	}
	if fnKw.MatchString(sig) {
		return false
	}
	return typeKw.MatchString(sig) || d.Kind == "type"
}

// private reports a declaration its language keeps out of the public
// surface: Go lower-case names, Rust without pub, Java and C# private,
// TypeScript not exported.
func private(d marker.Decl, sig, ext string) bool {
	name := lastPart(d.Symbol)
	switch ext {
	case ".go":
		if d.Kind == "method" {
			recv, _, _ := strings.Cut(d.Symbol, ".")
			return lower(recv) || lower(name)
		}
		return lower(name)
	case ".rs":
		return !strings.HasPrefix(sig, "pub")
	case ".java", ".kt", ".kts", ".cs", ".scala", ".php", ".swift":
		return strings.Contains(" "+sig, " private ") || strings.Contains(" "+sig, " fileprivate ")
	case ".ts", ".tsx", ".mts":
		if d.Parent != "" {
			return strings.Contains(" "+sig, " private ") || strings.HasPrefix(sig, "#")
		}
		return !strings.HasPrefix(sig, "export")
	}
	return false
}

func lower(s string) bool {
	for _, r := range s {
		return unicode.IsLower(r)
	}
	return false
}

var testDecorator = regexp.MustCompile(`^(@Test\b|@ParameterizedTest\b|@RepeatedTest\b|@TestFactory\b|\[Test\b|\[Fact\b|\[Theory\b|\[TestMethod\b|\[TestCase\b|#\[test\]|#\[tokio::test|#\[rstest|@pytest\.mark\.)`)

// isTest reports a declaration that is a test (or a whole test class), as
// opposed to a fake, a stub, a fixture or a builder sitting beside it.
func isTest(d marker.Decl, sig, path string) bool {
	if strings.HasSuffix(path, ".feature") {
		return strings.HasPrefix(d.Symbol, "feature:") || strings.HasPrefix(d.Symbol, "scenario")
	}
	for _, dec := range d.Decorators {
		if strings.HasPrefix(dec, "@pytest.fixture") || strings.HasPrefix(dec, "@fixture") || strings.HasPrefix(dec, "@BeforeEach") || strings.HasPrefix(dec, "@Before") {
			return false
		}
		if testDecorator.MatchString(dec) {
			return true
		}
	}
	name := lastPart(d.Symbol)
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".go":
		for _, p := range []string{"Test", "Benchmark", "Fuzz", "Example"} {
			if strings.HasPrefix(name, p) && d.Kind == "function" && (len(name) == len(p) || !lower(name[len(p):])) {
				return true
			}
		}
		return false
	case ".py":
		if d.Kind == "class" {
			return strings.HasPrefix(name, "Test")
		}
		return strings.HasPrefix(name, "test")
	case ".rb":
		return strings.HasPrefix(name, "test_") || strings.HasSuffix(name, "Test")
	}
	if typeLike(d, sig) {
		return strings.HasSuffix(name, "Test") || strings.HasSuffix(name, "Tests") || strings.HasSuffix(name, "Spec") || strings.HasSuffix(name, "IT")
	}
	return strings.HasPrefix(name, "test") && len(name) > 4 && !lower(name[4:5])
}

// testSubject strips the test affixes from a test's name: test_renewal,
// TestRenewal, RenewalTest and RenewalSpec all name Renewal.
func testSubject(name string) string {
	for _, p := range []string{"test_", "Test_", "Test", "test", "Benchmark", "Fuzz", "Example"} {
		if strings.HasPrefix(name, p) && len(name) > len(p) {
			name = strings.TrimPrefix(name, p)
			break
		}
	}
	for _, s := range []string{"_test", "Tests", "Test", "Spec", "IT"} {
		if strings.HasSuffix(name, s) && len(name) > len(s) {
			name = strings.TrimSuffix(name, s)
			break
		}
	}
	return strings.TrimLeft(name, "_")
}
