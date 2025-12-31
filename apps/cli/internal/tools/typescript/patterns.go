package typescript

import (
	"regexp"
)

// TypeScript error patterns for tsc output.
// The TypeScript compiler uses parentheses for line/column: file.ts(line,col): error TSxxxx: message

var (
	// tsErrorPattern matches TypeScript compiler errors.
	// Format: file.ts(line,col): error TSxxxx: message
	// Or without error code: file.ts(line,col): message
	//
	// The pattern must handle:
	// - Relative paths: src/app.ts, components/Button.tsx
	// - Absolute Unix paths: /home/user/project/app.ts
	// - Absolute Windows paths: C:\Users\project\app.ts
	// - Paths with dots/dashes/underscores: src/v2.0/my-component_test.tsx
	// - Declaration files: types/index.d.ts, globals.d.tsx
	// - ES Module/CommonJS TypeScript files: src/app.mts, src/app.cts
	//
	// Groups:
	//   1: file path (e.g., "src/app.ts", "components/Button.tsx", "types/index.d.ts")
	//   2: line number
	//   3: column number
	//   4: TS error code (optional, e.g., "TS2749")
	//   5: error message
	//
	// Pattern breakdown:
	// - ^([^\s(]+\.(?:d\.)?[cm]?tsx?)\( : file path ending in .ts, .tsx, .d.ts, .d.tsx, .mts, .cts, .mtsx, .ctsx
	//   Uses [^\s(]+ instead of .+ to prevent backtracking (no spaces or parens in path)
	// - (\d+),(\d+)\) : line,col numbers in parentheses
	// - :\s* : colon with optional whitespace
	// - (?:error\s+(TS\d+):\s*)? : optional "error TSxxxx: " prefix
	// - (.+?)\s*$ : the message (lazy match to avoid trailing whitespace)
	tsErrorPattern = regexp.MustCompile(`^([^\s(]+\.(?:d\.)?[cm]?tsx?)\((\d+),(\d+)\):\s*(?:error\s+(TS\d+):\s*)?(.+?)\s*$`)

	// noisePatterns are lines that should be skipped as TypeScript-specific noise
	noisePatterns = []*regexp.Regexp{
		// Watch mode output
		regexp.MustCompile(`^Starting compilation`),                  // tsc watch mode
		regexp.MustCompile(`^File change detected`),                  // tsc watch mode
		regexp.MustCompile(`^Watching for file changes`),             // tsc watch mode
		regexp.MustCompile(`^\[\d{1,2}:\d{2}:\d{2}\s*(AM|PM)?\]`),     // tsc timestamp prefix [HH:MM:SS AM/PM]

		// Summary and informational messages
		regexp.MustCompile(`^Found \d+ error`),                       // tsc error summary
		regexp.MustCompile(`^Version \d+\.\d+`),                      // tsc version output
		regexp.MustCompile(`^message TS\d+:`),                        // tsc informational messages (e.g., TS6194)

		// Build mode output (tsc -b / tsc --build)
		regexp.MustCompile(`^Projects in this build:`),               // tsc build mode project list
		regexp.MustCompile(`^\s*\* .+tsconfig.*\.json`),              // tsc build mode project entry
		regexp.MustCompile(`^Building project`),                      // tsc build mode building
		regexp.MustCompile(`^Project '.+' is out of date`),           // tsc build mode stale project
		regexp.MustCompile(`^Project '.+' is up to date`),            // tsc build mode fresh project
		regexp.MustCompile(`^Updating output timestamps`),            // tsc build mode timestamps
		regexp.MustCompile(`^Skipping build of project`),             // tsc build mode skip

		// Pretty output noise (code snippets and underlines)
		regexp.MustCompile(`^\s+\d+\s*\|`),                           // Pretty output line numbers with pipe
		regexp.MustCompile(`^\s+\|`),                                 // Pretty output continuation lines
		regexp.MustCompile(`^\s+~+\s*$`),                             // Pretty output error underlines

		// Empty and whitespace
		regexp.MustCompile(`^\s*$`),                                  // Empty lines
	}

	// TSErrorCategories maps TypeScript error code ranges to categories.
	// Based on TypeScript compiler error codes:
	// https://github.com/microsoft/TypeScript/blob/main/src/compiler/diagnosticMessages.json
	TSErrorCategories = map[string]string{
		// TS1xxx: Syntax/parser errors
		"TS1": "syntax",
		// TS2xxx: Semantic/type errors (most common)
		"TS2": "type",
		// TS3xxx: Module/namespace errors (removed in later TS, rare)
		"TS3": "module",
		// TS4xxx: Declaration emit errors
		"TS4": "declaration",
		// TS5xxx: Compiler options errors
		"TS5": "config",
		// TS6xxx: Build mode and compilation messages
		"TS6": "build",
		// TS7xxx: noImplicitAny and related strict mode errors
		"TS7": "strict",
		// TS8xxx: JSX errors
		"TS8": "jsx",
		// TS9xxx: Other compiler messages
		"TS9": "compiler",
		// TS17xxx: Advanced features errors (decorators, etc.)
		"TS17": "advanced",
		// TS18xxx: Additional semantic errors
		"TS18": "semantic",
	}

	// CommonTSErrors maps common TypeScript error codes to descriptions.
	// This helps with error message enhancement and categorization.
	CommonTSErrors = map[string]string{
		// Type assignment errors
		"TS2322": "Type is not assignable",
		"TS2345": "Argument type is not assignable to parameter type",
		"TS2339": "Property does not exist on type",
		"TS2304": "Cannot find name",
		"TS2305": "Module has no exported member",
		"TS2307": "Cannot find module",
		"TS2314": "Generic type requires type arguments",
		"TS2315": "Type is not generic",
		"TS2349": "Cannot invoke expression",
		"TS2352": "Conversion may be a mistake",
		"TS2353": "Object literal may only specify known properties",
		"TS2355": "Function must return a value",
		"TS2365": "Operator cannot be applied to types",
		"TS2366": "Function lacks ending return statement",
		"TS2367": "Comparison will always be false",
		"TS2531": "Object is possibly null",
		"TS2532": "Object is possibly undefined",
		"TS2533": "Object is possibly null or undefined",
		"TS2551": "Property does not exist (did you mean...)",
		"TS2554": "Expected arguments but got different count",
		"TS2555": "Expected at least N arguments",
		"TS2556": "Expected at most N arguments",
		"TS2564": "Property has no initializer",
		"TS2571": "Object is of type unknown",
		"TS2590": "Expression produces union too complex",
		"TS2614": "Module has no default export",
		"TS2684": "The this context of type is not assignable",
		"TS2688": "Cannot find type definition file",
		"TS2693": "Type only refers to a type but is used as a value",
		"TS2717": "Subsequent property declarations must have same type",
		"TS2724": "Module has no exported member (did you mean...)",
		"TS2749": "Value refers to a type but is used as a value",
		"TS2769": "No overload matches this call",
		"TS2786": "Component cannot be used as JSX component",
		"TS2792": "Cannot find module (using custom conditions)",

		// Syntax errors
		"TS1002": "Unterminated string literal",
		"TS1003": "Identifier expected",
		"TS1005": "Token expected",
		"TS1009": "Trailing comma not allowed",
		"TS1054": "Return type annotation missing",
		"TS1109": "Expression expected",
		"TS1128": "Declaration or statement expected",
		"TS1136": "Property assignment expected",
		"TS1138": "Parameter declaration expected",
		"TS1141": "String literal expected",
		"TS1155": "const declarations must be initialized",
		"TS1160": "Unterminated template literal",
		"TS1161": "Unterminated regular expression",
		"TS1183": "Implementation cannot be declared in ambient contexts",
		"TS1184": "Modifiers cannot appear here",
		"TS1192": "Module has no default export",

		// Strict mode errors
		"TS7005": "Variable implicitly has an any type",
		"TS7006": "Parameter implicitly has an any type",
		"TS7008": "Member implicitly has an any type",
		"TS7010": "Function implicitly has return type any",
		"TS7015": "Element implicitly has an any type",
		"TS7016": "Could not find declaration file for module",
		"TS7017": "Element implicitly has an any type (index signature)",
		"TS7022": "Variable implicitly has type any in some locations",
		"TS7023": "Variable implicitly has return type any in some locations",
		"TS7030": "Not all code paths return a value",
		"TS7031": "Binding element implicitly has an any type",
		"TS7034": "Variable implicitly has type any in some locations (no best common type)",

		// Declaration errors
		"TS4053": "Return type of public method from exported class has or is using private name",
		"TS4055": "Return type of public method from exported class has or is using private name",
		"TS4058": "Return type of exported function has or is using private name",
		"TS4060": "Return type of public property getter from exported class has or is using private name",
		"TS4082": "Default export of module has or is using private name",
		"TS4112": "This member cannot have an override modifier because its containing class does not extend another class",
		"TS4113": "This member cannot have an override modifier because it is not declared in the base class",
		"TS4114": "This member must have an override modifier because it overrides a member in the base class",

		// Config/compiler options errors (TS5xxx)
		"TS5001": "The current host does not support the option",
		"TS5023": "Unknown compiler option",
		"TS5024": "Compiler option requires a value of type",
		"TS5053": "Option can only be used when module is set to",
		"TS5055": "Cannot write file because it would overwrite input file",
		"TS5056": "Cannot write file because it would be overwritten by multiple input files",
		"TS5069": "Option is not allowed when module is set to",
		"TS5083": "Cannot read file",
		"TS5097": "An import path can only end with .ts extension when allowImportingTsExtensions is enabled",

		// Build/project errors (TS6xxx)
		"TS6053": "File not found",
		"TS6059": "File is not under rootDir",
		"TS6133": "Variable is declared but its value is never read",
		"TS6138": "Property is declared but its value is never read",
		"TS6192": "All imports in import declaration are unused",
		"TS6196": "Variable is implicitly of type any because it does not have a type annotation",
		"TS6198": "All destructured elements are unused",
		"TS6199": "All variables are unused",
		"TS6305": "Output file has not been built from source file",
		"TS6306": "Referenced project may not disable emit",
		"TS6307": "Cannot prepend project to itself",
		"TS6310": "Referenced project must have setting composite: true",

		// Additional type errors
		"TS2300": "Duplicate identifier",
		"TS2318": "Cannot find global type",
		"TS2344": "Type does not satisfy constraint",
		"TS2362": "Left side of arithmetic operation must be of type any, number, bigint or an enum type",
		"TS2363": "Right side of arithmetic operation must be of type any, number, bigint or an enum type",
		"TS2395": "Individual declarations in merged declaration must be all exported or all local",
		"TS2420": "Class incorrectly implements interface",
		"TS2430": "Interface incorrectly extends interface",
		"TS2451": "Cannot redeclare block-scoped variable",
		"TS2454": "Variable is used before being assigned",
		"TS2488": "Type must have a Symbol.iterator method that returns an iterator",
		"TS2503": "Cannot find namespace",
		"TS2504": "Type must implement method for iterator",
		"TS2515": "Non-abstract class does not implement inherited abstract member",
		"TS2525": "Initializer provides no value for this binding element",
		"TS2538": "Type cannot be used as an index type",
		"TS2540": "Cannot assign to read-only property",
		"TS2552": "Cannot find name (did you mean...)",
		"TS2559": "Type has no properties in common with type",
		"TS2560": "Value of type has no properties in common with type",
		"TS2568": "Property may not exist on type - consider using optional chaining",
		"TS2580": "Cannot find name, possibly need to install type definitions",
		"TS2584": "Cannot find name, need to change target library",
		"TS2588": "Cannot assign to because it is a constant",
		"TS2591": "Cannot find name, possibly missing DOM library",
		"TS2602": "JSX element implicitly has type any",
		"TS2604": "JSX element type does not have any construct or call signatures",
		"TS2607": "JSX element class does not support attributes",
		"TS2709": "Cannot use namespace as a type",
		"TS2722": "Cannot invoke an object which is possibly undefined",
		"TS2739": "Type is missing properties from type",
		"TS2741": "Property is missing in type but required in type",
		"TS2774": "This condition will always return true",
		"TS2775": "Assertions require every name in the call target to be declared with an explicit type annotation",
		"TS2790": "Operand of delete must be optional",
		"TS2794": "Expected at least N arguments, but got N",
		"TS2802": "Type can only be iterated through when using --downlevelIteration",
		"TS2820": "Type is not assignable to type (strictNullChecks)",

		// JSX errors (TS8xxx)
		"TS8006": "An implementation cannot be declared in ambient contexts",
	}
)
