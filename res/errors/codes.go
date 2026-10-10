package errors

type ErrorCode struct {
	Code        string
	Message     string
	Description string
	Tip         string
	Example     string // опционально
}

var ErrorCodes = map[string]ErrorCode{
	// ============================================================================
	// 0000: Unknown
	// ============================================================================
	"0000": {
		Code:        "0000",
		Message:     "Unknown error",
		Description: "An unknown error occurred. This usually means a compiler bug — some error path did not set a specific code.",
		Tip:         "Please report this bug with the full output.",
	},

	// ============================================================================
	// 0010-0029: Compiler / Internal / Config
	// ============================================================================
	"0010": {
		Code:        "0010",
		Message:     "Cannot read config file",
		Description: "The manifest.spc file could not be read. It might be missing, or the path is wrong.",
		Tip:         "Check that manifest.spc exists in the project root and is readable.",
		Example:     "Project dir should contain: manifest.spc",
	},
	"0011": {
		Code:        "0011",
		Message:     "File not found",
		Description: "The specified source file could not be found.",
		Tip:         "Check that the file path in manifest.spc (main=...) is correct.",
	},
	"0020": {
		Code:        "0020",
		Message:     "Cannot write C file",
		Description: "Failed to write the generated C code to disk. Check write permissions.",
		Tip:         "Check write permissions for the output directory.",
	},
	"0021": {
		Code:        "0021",
		Message:     "Compilation failed",
		Description: "The C compiler returned an error while compiling the generated C code. This usually indicates a backend bug.",
		Tip:         "Check the C compiler output above. If the C code looks wrong, report a bug.",
	},
	"0022": {
		Code:        "0022",
		Message:     "Cannot create output directory",
		Description: "Failed to create the build output directory.",
		Tip:         "Check permissions and the buildOutPath in manifest.spc.",
	},

	// ============================================================================
	// 0100-0199: Lexer
	// ============================================================================
	"0100": {
		Code:        "0100",
		Message:     "Unknown character",
		Description: "The lexer encountered a character it doesn't recognize.",
		Tip:         "Check for typos or invalid symbols in your code.",
	},
	"0101": {
		Code:        "0101",
		Message:     "Unknown character '|'",
		Description: "A single '|' is not valid in Skorpion. Did you mean '||' (logical OR)?",
		Tip:         "Use '||' for logical OR, or '|' is not supported.",
		Example:     "// Wrong:\nif (a | b) { ... }\n\n// Right:\nif (a || b) { ... }",
	},
	"0102": {
		Code:        "0102",
		Message:     "Unterminated string",
		Description: "A string literal was not closed with a closing quote before the end of line.",
		Tip:         "Add a closing '\"' to the string.",
		Example:     "// Wrong:\nstring s = \"hello\n\n// Right:\nstring s = \"hello\"",
	},
	"0103": {
		Code:        "0103",
		Message:     "Unterminated tildes",
		Description: "A ~~~ block (includeC) was not closed. The lexer reached the end of file without finding the closing ~~~.",
		Tip:         "Add closing ~~~ after the C code.",
		Example:     "includeC ~~~\n  printf(\"hi\");\n~~~",
	},
	"0104": {
		Code:        "0104",
		Message:     "Unterminated multi-line comment",
		Description: "A /* comment was not closed with */.",
		Tip:         "Add '*/' to close the comment.",
	},
	"0105": {
		Code:    "0105",
		Message: "Unknown escape sequence",
		Description: "An unrecognized escape sequence was used in a string literal. " +
			"The backslash and the following character are kept as-is.",
		Tip: "If you meant a literal backslash, use '\\\\'. " +
			"If you meant a special character, use one of: \\n, \\t, \\r, \\0, \\\\, \\\", \\'.",
		Example: "// Warning:\nstring path = \"C:\\q\"   // keeps '\\q' as-is\n\n// To silence:\nstring path = \"C:\\\\q\"  // literal backslash",
	},
	"0106": {
		Code:        "0106",
		Message:     "Invalid number format",
		Description: "The number literal is malformed (e.g. two dots).",
		Tip:         "Check the number format: integers like '42', floats like '3.14'.",
		Example:     "// Wrong:\nint x = 1.2.3\n\n// Right:\nint x = 1",
	},

	// ============================================================================
	// 0500-0699: Parser
	// ============================================================================

	// --- 0500: Generic ---
	"0500": {
		Code:        "0500",
		Message:     "Unexpected token",
		Description: "The parser found a token that does not fit in the current context.",
		Tip:         "Check the syntax around this location.",
	},

	// --- 0501-0510: Function parsing ---
	"0501": {
		Code:        "0501",
		Message:     "Expected function name",
		Description: "After a return type, a function name is expected.",
		Tip:         "Add a function name: 'void myFunc() { ... }'.",
		Example:     "// Wrong:\nvoid (arr args) { }\n\n// Right:\nvoid main(arr args) { }",
	},
	"0502": {
		Code:        "0502",
		Message:     "Expected parameter type",
		Description: "A function parameter must start with a type.",
		Tip:         "Use: 'void func(int x, string s) { ... }'.",
		Example:     "// Wrong:\nvoid func(x, y) { }\n\n// Right:\nvoid func(int x, int y) { }",
	},
	"0503": {
		Code:        "0503",
		Message:     "Expected parameter name",
		Description: "After a parameter type, a name is expected.",
		Tip:         "Use: 'void func(int x) { ... }'.",
	},
	"0504": {
		Code:        "0504",
		Message:     "Expected variable name",
		Description: "After a type, a variable or field name is expected.",
		Tip:         "Use: 'int x = 10'.",
	},
	"0505": {
		Code:        "0505",
		Message:     "Unexpected keyword in expression",
		Description: "A keyword appeared where an expression was expected.",
		Tip:         "Check the expression syntax. Keywords like 'func', 'if', 'while' cannot be used as values.",
	},
	"0506": {
		Code:        "0506",
		Message:     "Unexpected token in expression",
		Description: "The parser found an unexpected token inside an expression.",
		Tip:         "Check the expression syntax around this location.",
	},
	"0507": {
		Code:        "0507",
		Message:     "Expected '('",
		Description: "An opening parenthesis was expected here.",
		Tip:         "Add '(' at this location.",
		Example:     "// Wrong:\nvoid main arr args) { }\n\n// Right:\nvoid main(arr args) { }",
	},
	"0508": {
		Code:        "0508",
		Message:     "Expected ')'",
		Description: "A closing parenthesis was expected here.",
		Tip:         "Add ')' at this location.",
		Example:     "// Wrong:\nvoid main(arr args { }\n\n// Right:\nvoid main(arr args) { }",
	},
	"0509": {
		Code:        "0509",
		Message:     "Expected '{'",
		Description: "An opening brace was expected here. Usually means a block or function body is missing its '{'.",
		Tip:         "Add '{' at this location.",
		Example:     "// Wrong:\nvoid main(arr args)\n    sendln(\"hi\")\n}\n\n// Right:\nvoid main(arr args) {\n    sendln(\"hi\")\n}",
	},
	"0510": {
		Code:        "0510",
		Message:     "Expected ~~~ after includeC",
		Description: "includeC must be followed by a ~~~ block containing C code.",
		Tip:         "Use: includeC ~~~ ... ~~~.",
	},

	// --- 0512-0519: Statements ---
	"0512": {
		Code:        "0512",
		Message:     "Expected ',' after case branch",
		Description: "Each case branch must end with a comma.",
		Tip:         "Add ',' after the branch body.",
		Example:     "case (x) {\n  1 { sendln(\"one\") },\n  2 { sendln(\"two\") },\n  _ { sendln(\"other\") },\n}",
	},
	"0513": {
		Code:        "0513",
		Message:     "Expected type in arr[]",
		Description: "arr[] requires an element type inside the brackets.",
		Tip:         "Use: 'arr[int]', 'arr[string]', etc.",
		Example:     "// Wrong:\narr[] x = [1, 2]\n\n// Right:\narr[int] x = [1, 2]",
	},
	"0514": {
		Code:        "0514",
		Message:     "Expected ']' in array type",
		Description: "The array type was not closed with ']'.",
		Tip:         "Add ']' after the element type.",
		Example:     "// Wrong:\narr[int x = [1, 2]\n\n// Right:\narr[int] x = [1, 2]",
	},
	"0515": {
		Code:        "0515",
		Message:     "Expected ']'",
		Description: "A closing bracket was expected here.",
		Tip:         "Add ']' at this location.",
	},
	"0516": {
		Code:        "0516",
		Message:     "Expected ']' in array index",
		Description: "The array index was not closed with ']'.",
		Tip:         "Add ']' after the index expression.",
		Example:     "// Wrong:\nint x = a[0\n\n// Right:\nint x = a[0]",
	},
	"0517": {
		Code:        "0517",
		Message:     "Expected '}'",
		Description: "A closing brace was expected here.",
		Tip:         "Add '}' at this location.",
		Example:     "// Wrong:\nvoid main(arr args) {\n    sendln(\"hi\")\n\n// Right:\nvoid main(arr args) {\n    sendln(\"hi\")\n}",
	},
	"0518": {
		Code:        "0518",
		Message:     "Expected '['",
		Description: "An opening bracket was expected here.",
		Tip:         "Add '[' at this location.",
	},
	"0519": {
		Code:        "0519",
		Message:     "try without catch",
		Description: "A 'try' block must have at least one 'catch' clause.",
		Tip:         "Add 'catch (...) { ... }' after the try block.",
		Example:     "try {\n  f()\n} catch (Error as e) {\n  sendln(e.msg)\n}",
	},

	// --- 0520-0529: Specific parsing ---
	"0520": {
		Code:        "0520",
		Message:     "Expected method name after '.'",
		Description: "After '.', a method or field name is expected.",
		Tip:         "Use: 'obj.method()' or 'obj.field'.",
	},
	"0521": {
		Code:        "0521",
		Message:     "Expected ':' in ternary",
		Description: "A ternary expression requires ':' between the two branches.",
		Tip:         "Use: 'cond ? a : b'.",
		Example:     "// Wrong:\nint x = a > 0 ? a b\n\n// Right:\nint x = a > 0 ? a : b",
	},
	"0522": {
		Code:        "0522",
		Message:     "Empty case statement",
		Description: "A case statement must have at least one branch.",
		Tip:         "Add a branch or remove the case.",
	},
	"0523": {
		Code:        "0523",
		Message:     "Unexpected end of file",
		Description: "The parser reached the end of the file unexpectedly. Usually a missing closing brace or bracket.",
		Tip:         "Check for missing closing braces or brackets.",
	},
	"0524": {
		Code:        "0524",
		Message:     "Expected '..' in range",
		Description: "A range expression requires '..' between start and end.",
		Tip:         "Use: '1..5' for range 1 to 5.",
		Example:     "// Wrong:\narr[int] x = (1, 5)\n\n// Right:\narr[int] x = (1..5)",
	},
	"0525": {
		Code:        "0525",
		Message:     "Expected identifier",
		Description: "An identifier (name) was expected here.",
		Tip:         "Add a valid identifier name.",
	},

	// --- 0540-0552: Token expectation (from parser.expect) ---
	"0540": {
		Code:        "0540",
		Message:     "Expected '('",
		Description: "An opening parenthesis was expected.",
		Tip:         "Add '(' at this location.",
	},
	"0541": {
		Code:        "0541",
		Message:     "Expected ')'",
		Description: "A closing parenthesis was expected.",
		Tip:         "Add ')' at this location.",
	},
	"0542": {
		Code:        "0542",
		Message:     "Expected '{'",
		Description: "An opening brace was expected.",
		Tip:         "Add '{' at this location.",
	},
	"0543": {
		Code:        "0543",
		Message:     "Expected '}'",
		Description: "A closing brace was expected.",
		Tip:         "Add '}' at this location.",
	},
	"0544": {
		Code:        "0544",
		Message:     "Expected '['",
		Description: "An opening bracket was expected.",
		Tip:         "Add '[' at this location.",
	},
	"0545": {
		Code:        "0545",
		Message:     "Expected ']'",
		Description: "A closing bracket was expected.",
		Tip:         "Add ']' at this location.",
	},
	"0546": {
		Code:        "0546",
		Message:     "Expected ':'",
		Description: "A colon was expected here.",
		Tip:         "Add ':' at this location.",
	},
	"0547": {
		Code:        "0547",
		Message:     "Expected ';'",
		Description: "A semicolon was expected here.",
		Tip:         "Add ';' at this location.",
	},
	"0548": {
		Code:        "0548",
		Message:     "Expected ','",
		Description: "A comma was expected here.",
		Tip:         "Add ',' at this location.",
	},
	"0549": {
		Code:        "0549",
		Message:     "Expected '='",
		Description: "An equals sign was expected here.",
		Tip:         "Add '=' at this location.",
	},
	"0550": {
		Code:        "0550",
		Message:     "Expected '..'",
		Description: "A range operator '..' was expected here.",
		Tip:         "Add '..' at this location.",
	},
	"0551": {
		Code:        "0551",
		Message:     "Expected identifier",
		Description: "An identifier was expected here.",
		Tip:         "Add a valid identifier name.",
	},
	"0552": {
		Code:        "0552",
		Message:     "Expected expression",
		Description: "An expression was expected here.",
		Tip:         "Add an expression (number, string, variable, call, ...).",
	},

	// --- 0553-0559: Imports / Functions / Variables ---
	"0553": {
		Code:        "0553",
		Message:     "Expected module path after 'use'",
		Description: "'use' must be followed by a module path.",
		Tip:         "Use: 'use std/io' or 'use #std/io'.",
		Example:     "// Wrong:\nuse\n\n// Right:\nuse #std/io",
	},
	"0554": {
		Code:        "0554",
		Message:     "Expected '#' or module name after 'use'",
		Description: "After 'use', either '#' (import all) or a module name is expected.",
		Tip:         "Use: 'use #std/io' or 'use std/io'.",
	},
	"0555": {
		Code:        "0555",
		Message:     "Expected ',' or ')' in parameter list",
		Description: "In a function parameter list, parameters must be separated by ','.",
		Tip:         "Add ',' between parameters.",
		Example:     "// Wrong:\nvoid f(int x int y) { }\n\n// Right:\nvoid f(int x, int y) { }",
	},
	"0556": {
		Code:        "0556",
		Message:     "Default value before required parameter",
		Description: "A parameter with a default value cannot come before a parameter without one.",
		Tip:         "Move all parameters with defaults to the end.",
		Example:     "// Wrong:\nvoid f(int x = 1, int y) { }\n\n// Right:\nvoid f(int y, int x = 1) { }",
	},
	"0557": {
		Code:        "0557",
		Message:     "Expected '=' or ';' after variable name",
		Description: "After a variable name, either '=' (initializer) or ';' (end of declaration) is expected.",
		Tip:         "Add '=' or ';'.",
	},
	"0558": {
		Code:        "0558",
		Message:     "Expected expression after '='",
		Description: "An initializer expression was expected after '='.",
		Tip:         "Add an expression after '='.",
		Example:     "// Wrong:\nint x =\n\n// Right:\nint x = 5",
	},
	"0559": {
		Code:        "0559",
		Message:     "Expected ';' after variable declaration",
		Description: "A variable declaration must end with ';'.",
		Tip:         "Add ';' at the end of the declaration.",
	},

	// --- 0560-0562: Array types ---
	"0560": {
		Code:        "0560",
		Message:     "Array element type cannot be 'void'",
		Description: "An array of 'void' makes no sense — all elements would be null.",
		Tip:         "Use a concrete element type: arr[int], arr[string], etc.",
		Example:     "// Wrong:\narr[void] x = [null, null]\n\n// Right:\narr[int] x = [1, 2]",
	},
	"0561": {
		Code:        "0561",
		Message:     "Unknown element type in arr[]",
		Description: "The type inside arr[] is not a recognized type.",
		Tip:         "Use one of: int, string, float, double, bool, char, any, arr[...].",
	},
	"0562": {
		Code:        "0562",
		Message:     "Expected index expression",
		Description: "An array index must contain an expression.",
		Tip:         "Add an index expression inside the brackets.",
		Example:     "// Wrong:\nint x = a[]\n\n// Right:\nint x = a[0]",
	},

	// --- 0563-0568: Conditions ---
	"0563": {
		Code:        "0563",
		Message:     "Expected condition after 'if'",
		Description: "'if' must be followed by a condition.",
		Tip:         "Add a condition: 'if x > 0 { ... }'.",
	},
	"0564": {
		Code:        "0564",
		Message:     "Expected condition after 'while'",
		Description: "'while' must be followed by a condition.",
		Tip:         "Add a condition: 'while x > 0 { ... }'.",
	},
	"0565": {
		Code:        "0565",
		Message:     "Expected condition after 'elsif'",
		Description: "'elsif' must be followed by a condition.",
		Tip:         "Add a condition: 'elsif x < 0 { ... }'.",
	},
	"0566": {
		Code:        "0566",
		Message:     "Expected '(' after 'for'",
		Description: "'for' must be followed by '(' containing init; cond; post.",
		Tip:         "Use: 'for (init; cond; post) { ... }'.",
	},
	"0567": {
		Code:        "0567",
		Message:     "Expected ')' after for clauses",
		Description: "The for-loop header was not closed with ')'.",
		Tip:         "Add ')' after the post clause.",
	},
	"0568": {
		Code:        "0568",
		Message:     "Expected two ';' in for",
		Description: "A for-loop requires exactly two semicolons: 'init; cond; post'.",
		Tip:         "Use: 'for (init; cond; post) { ... }'.",
		Example:     "// Wrong:\nfor (int i = 0; i < 10) { }\n\n// Right:\nfor (int i = 0; i < 10; i = i + 1) { }",
	},

	// --- 0569-0573: Case ---
	"0569": {
		Code:        "0569",
		Message:     "Expected '(' after 'case'",
		Description: "'case' must be followed by '(' and a value.",
		Tip:         "Use: 'case (value) { ... }'.",
	},
	"0570": {
		Code:        "0570",
		Message:     "Expected ')' after case value",
		Description: "The case value expression was not closed with ')'.",
		Tip:         "Add ')' after the case value.",
	},
	"0571": {
		Code:        "0571",
		Message:     "Expected '{' after case",
		Description: "The case statement body was not opened with '{'.",
		Tip:         "Add '{' after the case value.",
	},
	"0572": {
		Code:        "0572",
		Message:     "Expected ',' after default branch",
		Description: "The default ('_') branch must end with ',' like other branches.",
		Tip:         "Add ',' after the default branch body.",
	},
	"0573": {
		Code:        "0573",
		Message:     "Default branch must be last",
		Description: "The '_' branch must be the last branch in a case statement.",
		Tip:         "Move '_' to the end, or remove it.",
	},

	// --- 0574-0578: Try/Catch ---
	"0574": {
		Code:        "0574",
		Message:     "Expected '{' after 'try'",
		Description: "The try block body was not opened with '{'.",
		Tip:         "Add '{' after 'try'.",
		Example:     "try {\n  f()\n} catch (Error as e) {\n  sendln(e.msg)\n}",
	},
	"0575": {
		Code:        "0575",
		Message:     "Expected '{' after 'catch'",
		Description: "The catch block body was not opened with '{'.",
		Tip:         "Add '{' after the catch clause.",
	},
	"0576": {
		Code:        "0576",
		Message:     "Expected error type in catch",
		Description: "After 'catch', an error type name is expected.",
		Tip:         "Use: 'catch (MyError as e) { ... }'.",
	},
	"0577": {
		Code:        "0577",
		Message:     "Expected variable name after 'as'",
		Description: "After 'as' in a catch clause, a variable name is expected.",
		Tip:         "Use: 'catch (MyError as e) { ... }'.",
	},
	"0578": {
		Code:        "0578",
		Message:     "Expected ')' after catch clause",
		Description: "The catch clause header was not closed with ')'.",
		Tip:         "Add ')' after the catch clause.",
		Example:     "// Wrong:\ncatch (MyError as e { }\n\n// Right:\ncatch (MyError as e) { }",
	},

	// --- 0579: Throw ---
	"0579": {
		Code:        "0579",
		Message:     "throw requires an expression",
		Description: "'throw' must be followed by an error instance or error variable.",
		Tip:         "Use: 'throw MyError{msg: \"...\"}' or 'throw err'.",
	},

	// --- 0580-0590: Error declaration ---
	"0580": {
		Code:        "0580",
		Message:     "Expected error type name after 'const'",
		Description: "'const' for an error type must be followed by a name.",
		Tip:         "Use: 'const MyError{...} = new Error'.",
	},
	"0581": {
		Code:        "0581",
		Message:     "Error type name must start with uppercase",
		Description: "Error type names must be capitalized by convention.",
		Tip:         "Rename the type to start with an uppercase letter.",
		Example:     "// Wrong:\nconst myError{msg: string} = new Error\n\n// Right:\nconst MyError{msg: string} = new Error",
	},
	"0582": {
		Code:        "0582",
		Message:     "Expected '{' after error type name",
		Description: "After the error type name, '{' with fields is expected.",
		Tip:         "Add '{' after the type name.",
	},
	"0583": {
		Code:        "0583",
		Message:     "Expected field name",
		Description: "Inside the error type field list, a field name is expected.",
		Tip:         "Use: '{ msg: string }'.",
	},
	"0584": {
		Code:        "0584",
		Message:     "Expected ':' after field name",
		Description: "After a field name, ':' and a type are expected.",
		Tip:         "Use: '{ msg: string }'.",
	},
	"0585": {
		Code:        "0585",
		Message:     "Expected field type",
		Description: "After ':', a valid type is expected.",
		Tip:         "Use a valid type: int, string, float, double, bool, char, arr, any.",
	},
	"0586": {
		Code:        "0586",
		Message:     "Expected ']' after default value",
		Description: "The default value in '[value]' was not closed with ']'.",
		Tip:         "Add ']' after the default value.",
	},
	"0587": {
		Code:        "0587",
		Message:     "Expected '}' after error fields",
		Description: "The error type field list was not closed with '}'.",
		Tip:         "Add '}' after the fields.",
	},
	"0588": {
		Code:        "0588",
		Message:     "Expected '=' after error fields",
		Description: "After the field list, '=' and a parent type are expected.",
		Tip:         "Use: 'const MyError{...} = new Error'.",
	},
	"0589": {
		Code:        "0589",
		Message:     "Expected parent error type",
		Description: "After '=', a parent error type name is expected.",
		Tip:         "Use: '= new Error' or '= SomeOtherError'.",
	},
	"0590": {
		Code:        "0590",
		Message:     "Expected 'new' or error type after '='",
		Description: "After '=', either 'new' or a parent error type name is expected.",
		Tip:         "Use: '= new Error' or '= ParentError'.",
	},

	// --- 0591-0594: Error instance ---
	"0591": {
		Code:        "0591",
		Message:     "Expected field name in error instance",
		Description: "Inside an error instance '{...}', a field name is expected.",
		Tip:         "Use: 'MyError{msg: \"...\"}'.",
	},
	"0592": {
		Code:        "0592",
		Message:     "Expected ':' after field name in instance",
		Description: "After a field name, ':' and a value are expected.",
		Tip:         "Use: 'MyError{msg: \"...\"}'.",
	},
	"0593": {
		Code:        "0593",
		Message:     "Expected '}' after error instance fields",
		Description: "The error instance field list was not closed with '}'.",
		Tip:         "Add '}' after the fields.",
	},
	"0594": {
		Code:        "0594",
		Message:     "Duplicate field in error instance",
		Description: "The same field name appears more than once in an error instance.",
		Tip:         "Remove the duplicate field.",
		Example:     "// Wrong:\nMyError{msg: \"a\", msg: \"b\"}\n\n// Right:\nMyError{msg: \"a\"}",
	},

	// --- 0595-0607: Expressions ---
	"0595": {
		Code:        "0595",
		Message:     "Expected '(' after method name",
		Description: "After a method name following '.', '(' is expected.",
		Tip:         "Add '(' after the method name.",
	},
	"0596": {
		Code:        "0596",
		Message:     "Expected field or method name after '.'",
		Description: "After '.', a field or method name (identifier) is expected.",
		Tip:         "Add a valid identifier after '.'.",
	},
	"0597": {
		Code:        "0597",
		Message:     "Unexpected keyword in statement position",
		Description: "A keyword appeared where a statement was expected.",
		Tip:         "Check the statement syntax.",
	},
	"0598": {
		Code:        "0598",
		Message:     "Expected statement",
		Description: "A statement was expected but none was found.",
		Tip:         "Add a statement or remove the empty construct.",
	},
	"0599": {
		Code:        "0599",
		Message:     "Expected ')' after expression",
		Description: "A parenthesized expression was not closed with ')'.",
		Tip:         "Add ')' at this location.",
		Example:     "// Wrong:\nint x = (1 + 2\n\n// Right:\nint x = (1 + 2)",
	},
	"0600": {
		Code:        "0600",
		Message:     "Expected ']' after array literal",
		Description: "An array literal was not closed with ']'.",
		Tip:         "Add ']' at this location.",
		Example:     "// Wrong:\narr[int] x = [1, 2\n\n// Right:\narr[int] x = [1, 2]",
	},
	"0601": {
		Code:        "0601",
		Message:     "Expected ',' or ']' in array literal",
		Description: "Array literal elements must be separated by ',' or the literal closed with ']'.",
		Tip:         "Add ',' between elements or ']' to close.",
		Example:     "// Wrong:\narr[int] x = [1 2]\n\n// Right:\narr[int] x = [1, 2]",
	},
	"0602": {
		Code:        "0602",
		Message:     "Expected ')' after arguments",
		Description: "The function call argument list was not closed with ')'.",
		Tip:         "Add ')' after the arguments.",
		Example:     "// Wrong:\nf(1, 2\n\n// Right:\nf(1, 2)",
	},
	"0603": {
		Code:        "0603",
		Message:     "Expected argument expression",
		Description: "A function call argument was expected but none was found.",
		Tip:         "Add an expression or remove the trailing ','.",
		Example:     "// Wrong:\nf(1, )\n\n// Right:\nf(1)",
	},
	"0604": {
		Code:        "0604",
		Message:     "Expected '..' in range",
		Description: "A range expression requires '..' between start and end.",
		Tip:         "Use: '1..5'.",
	},
	"0605": {
		Code:        "0605",
		Message:     "Expected range end expression",
		Description: "After '..', an end expression is expected.",
		Tip:         "Add an end expression: '1..5'.",
	},
	"0606": {
		Code:        "0606",
		Message:     "Invalid escape sequence",
		Description: "An unknown escape sequence in a string literal.",
		Tip:         "Use: \\n, \\t, \\r, \\0, \\\\, \\\", \\'.",
	},
	"0607": {
		Code:        "0607",
		Message:     "Invalid number format",
		Description: "The number literal is malformed.",
		Tip:         "Check the number format.",
	},
	"0608": {
		Code:        "0608",
		Message:     "Expected '<' after 'T'",
		Description: "The union type T must be followed by '<' and a list of types.",
		Tip:         "Use: 'T<int, float>'.",
	},
	"0609": {
		Code:        "0609",
		Message:     "Expected '>' after T<...>",
		Description: "The union type list was not closed with '>'.",
		Tip:         "Add '>' at the end: 'T<int, float>'.",
	},
	"0610": {
		Code:        "0610",
		Message:     "Empty T<...>",
		Description: "T<...> requires at least one type inside the angle brackets.",
		Tip:         "Use: 'T<int>', 'T<int, float>', etc.",
	},
	"0611": {
		Code:        "0611",
		Message:     "Nested T<...>",
		Description: "T<...> cannot contain another T<...> inside. Union of unions is not supported.",
		Tip:         "Use a single-level T<A, B, C> instead of T<T<A, B>, C>.",
	},
	"0612": {
		Code:        "0612",
		Message:     "Type not in union",
		Description: "The assigned value's type is not in the union's type list.",
		Tip:         "Use a type from the union, or extend the union.",
	},
	"0613": {
		Code:        "0613",
		Message:     "Operation not valid for union",
		Description: "The operation is not valid for any type combination in the union.",
		Tip:         "Check the operator and the union's type list.",
	},
	"0614": {
		Code:    "0614",
		Message: "Operation may fail at runtime",
		Description: "The operation is valid for some type combinations in the union, " +
			"but not all. If the wrong runtime type is encountered, the program will fail.",
		Tip: "Use detruncate(x) to narrow the type before the operation, or " +
			"extend the union to cover all combinations.",
	},
	"0615": {
		Code:        "0615",
		Message:     "Cannot use 'any' in union",
		Description: "The 'any' type is not allowed inside T<...>. A union must list concrete types.",
		Tip:         "Use T<int, string, ...> with concrete types, or use 'any' without a union.",
		Example:     "// Wrong:\nT<any, int> x\n\n// Right:\nT<int, string> x",
	},
	"0620": {
		Code:        "0620",
		Message:     "Expected 'f' after precision",
		Description: "The '^' operator requires 'f' after the precision number.",
		Tip:         "Use: '3.14^2f'.",
	},
	"0621": {
		Code:        "0621",
		Message:     "Invalid precision",
		Description: "The precision in '^Nf' must be a non-negative integer.",
		Tip:         "Use: '3.14^0f', '3.14^2f', etc.",
	},
	"0622": {
		Code:        "0622",
		Message:     "Precision too large",
		Description: "Precision in '^Nf' must be <= 20.",
		Tip:         "Use a smaller precision.",
	},
	"0623": {
		Code:        "0623",
		Message:     "Expected '}' in ^X{...}",
		Description: "The trim set in '^X{...}' was not closed with '}'.",
		Tip:         "Add '}' at the end: '^X{1,8}'.",
	},
	"0624": {
		Code:        "0624",
		Message:     "Empty trim set",
		Description: "The trim set in '^X...' must contain at least one digit.",
		Tip:         "Use: '^X9' or '^X{1,8}'.",
	},
	"0625": {
		Code:        "0625",
		Message:     "Invalid format specifier",
		Description: "After '^', either 'Nf' (precision) or 'X...' (trim) is expected.",
		Tip:         "Use: '^2f' or '^X9'.",
	},
	"0626": {
		Code:        "0626",
		Message:     "Format only for numbers",
		Description: "The '^' operator can only be applied to numeric types (int, float, double).",
		Tip:         "Use '^' only on numbers.",
	},

	// ============================================================================
	// 1500-1599: Semantic errors
	// ============================================================================
	"1500": {
		Code:        "1500",
		Message:     "Name already declared",
		Description: "A function or error type with this name already exists.",
		Tip:         "Rename it or remove the duplicate.",
	},
	"1501": {
		Code:        "1501",
		Message:     "No main() function found",
		Description: "Every Skorpion program needs a main() function.",
		Tip:         "Add 'void main(arr args) { ... }' to your program.",
	},
	"1502": {
		Code:        "1502",
		Message:     "main() must return void",
		Description: "The main function must have return type 'void'.",
		Tip:         "Change to 'void main(arr args) { ... }'.",
	},
	"1503": {
		Code:        "1503",
		Message:     "main() must take exactly one parameter",
		Description: "main() must take one parameter of type 'arr'.",
		Tip:         "Use: 'void main(arr args) { ... }'.",
	},
	"1504": {
		Code:        "1504",
		Message:     "main() parameter must be of type 'arr'",
		Description: "The parameter of main() must be 'arr'.",
		Tip:         "Use: 'void main(arr args) { ... }'.",
	},
	"1505": {
		Code:        "1505",
		Message:     "Variable already declared",
		Description: "A variable with this name already exists in this scope.",
		Tip:         "Rename the variable or use a different scope.",
	},
	"1506": {
		Code:        "1506",
		Message:     "Unknown type",
		Description: "The specified type is not recognized.",
		Tip:         "Valid types: int, string, float, double, bool, char, arr, dict, any, void.",
	},
	"1507": {
		Code:        "1507",
		Message:     "Type mismatch in variable declaration",
		Description: "The type of the value doesn't match the declared type.",
		Tip:         "Check the declared type and the initializer.",
	},
	"1508": {
		Code:        "1508",
		Message:     "Undefined variable",
		Description: "The variable has not been declared.",
		Tip:         "Declare the variable before using it.",
	},
	"1509": {
		Code:        "1509",
		Message:     "Cannot assign to constant",
		Description: "Constants cannot be reassigned.",
		Tip:         "Remove the assignment or declare a non-const variable.",
	},
	"1510": {
		Code:        "1510",
		Message:     "Type mismatch in assignment",
		Description: "The type of the value doesn't match the variable type.",
		Tip:         "Check the variable type and the assigned value.",
	},
	"1511": {
		Code:        "1511",
		Message:     "Arithmetic operation requires numeric types",
		Description: "Arithmetic can only be done on numbers.",
		Tip:         "Convert values with to_int(), to_float(), or to_double().",
	},
	"1512": {
		Code:        "1512",
		Message:     "Comparison requires numeric types",
		Description: "Comparison can only be done on numbers.",
		Tip:         "Convert values with to_int() before comparing.",
	},
	"1513": {
		Code:        "1513",
		Message:     "Invalid number",
		Description: "The number literal is not valid.",
		Tip:         "Check the number format.",
	},
	"1514": {
		Code:        "1514",
		Message:     "Undefined identifier",
		Description: "The identifier has not been declared.",
		Tip:         "Declare the identifier before using it.",
	},
	"1515": {
		Code:        "1515",
		Message:     "Cannot return value from void function",
		Description: "A void function cannot return a value.",
		Tip:         "Remove the return value or change the function's return type.",
	},
	"1516": {
		Code:        "1516",
		Message:     "Return type mismatch",
		Description: "The returned value type doesn't match the function's return type.",
		Tip:         "Check the function signature and the returned value.",
	},
	"1517": {
		Code:        "1517",
		Message:     "Expected return value",
		Description: "A non-void function must return a value.",
		Tip:         "Add a return value of the correct type.",
	},
	"1518": {
		Code:        "1518",
		Message:     "Undefined function",
		Description: "The called function has not been declared.",
		Tip:         "Check the function name and imports.",
	},
	"1519": {
		Code:        "1519",
		Message:     "Argument count mismatch",
		Description: "The function was called with the wrong number of arguments.",
		Tip:         "Check the function signature.",
	},
	"1520": {
		Code:        "1520",
		Message:     "Argument type mismatch",
		Description: "An argument's type doesn't match the parameter type.",
		Tip:         "Check the function signature and argument types.",
	},
	"1521": {
		Code:        "1521",
		Message:     "If condition must be boolean",
		Description: "The condition in an 'if' must be a bool.",
		Tip:         "Use a comparison like 'x > 0'.",
	},
	"1522": {
		Code:        "1522",
		Message:     "While condition must be boolean",
		Description: "The condition in a 'while' must be a bool.",
		Tip:         "Use a comparison like 'x > 0'.",
	},
	"1523": {
		Code:        "1523",
		Message:     "For condition must be boolean",
		Description: "The condition in a 'for' must be a bool.",
		Tip:         "Use a comparison like 'x > 0'.",
	},
	"1524": {
		Code:        "1524",
		Message:     "Elsif condition must be boolean",
		Description: "The condition in an 'elsif' must be a bool.",
		Tip:         "Use a comparison like 'x > 0'.",
	},
	"1525": {
		Code:        "1525",
		Message:     "Pattern type mismatch",
		Description: "A case pattern's type doesn't match the case value.",
		Tip:         "Use patterns of the same type as the case value.",
	},
	"1526": {
		Code:        "1526",
		Message:     "Operator '!' requires bool",
		Description: "Logical NOT can only be applied to booleans.",
		Tip:         "Use 'x == false' or convert the value to bool.",
	},
	"1527": {
		Code:        "1527",
		Message:     "Unary '-' requires numeric",
		Description: "Unary minus can only be applied to numbers.",
		Tip:         "Convert the value to a number first.",
	},
	"1528": {
		Code:        "1528",
		Message:     "Operator '&&'/'||' requires bool (left)",
		Description: "Logical operators can only be applied to booleans.",
		Tip:         "Use comparisons like 'x > 0 && y < 10'.",
	},
	"1529": {
		Code:        "1529",
		Message:     "Operator '&&'/'||' requires bool (right)",
		Description: "Logical operators can only be applied to booleans.",
		Tip:         "Use comparisons like 'x > 0 && y < 10'.",
	},
	"1530": {
		Code:        "1530",
		Message:     "Ternary condition must be bool",
		Description: "The condition in a ternary must be a bool.",
		Tip:         "Use 'cond ? a : b' where cond is bool.",
	},
	"1531": {
		Code:        "1531",
		Message:     "Ternary branches type mismatch",
		Description: "The two branches of a ternary must have the same type.",
		Tip:         "Make both branches return the same type.",
	},
	"1532": {
		Code:        "1532",
		Message:     "Type mismatch",
		Description: "The type of the value doesn't match the expected type.",
		Tip:         "Check the function signature and the value type.",
	},
	"1533": {
		Code:        "1533",
		Message:     "Range call requires constant bounds",
		Description: "A range used as function arguments must have constant bounds.",
		Tip:         "Use literal numbers: '(1..5).func()'.",
	},
	"1534": {
		Code:        "1534",
		Message:     "Array element type mismatch",
		Description: "An array element's type doesn't match the array's element type.",
		Tip:         "Check the array declaration and its elements.",
	},
	"1535": {
		Code:        "1535",
		Message:     "Function must return a value",
		Description: "A non-void function must end with a return statement.",
		Tip:         "Add 'return value;' at the end of the function.",
	},
	"1536": {
		Code:        "1536",
		Message:     "Duplicate parameter",
		Description: "The same parameter name appears more than once.",
		Tip:         "Rename one of the parameters.",
	},
	"1537": {
		Code:        "1537",
		Message:     "Default parameter before required parameter",
		Description: "A parameter with a default value cannot come before a parameter without one.",
		Tip:         "Move all parameters with defaults to the end.",
	},
	"1538": {
		Code:        "1538",
		Message:     "Cannot use void function as value",
		Description: "A function returning void cannot be used in an expression.",
		Tip:         "Change the function's return type or don't use its result.",
	},
	"1539": {
		Code:        "1539",
		Message:     "Not a function",
		Description: "The identifier is not a function and cannot be called.",
		Tip:         "Check the identifier name.",
	},
	"1540": {
		Code:        "1540",
		Message:     "Duplicate case pattern",
		Description: "The same pattern appears more than once in a case statement.",
		Tip:         "Remove the duplicate branch.",
	},
	"1541": {
		Code:        "1541",
		Message:     "Division by zero",
		Description: "A literal zero was used as the divisor.",
		Tip:         "Check the divisor.",
	},
	"1542": {
		Code:        "1542",
		Message:     "Null in heterogeneous array",
		Description: "Cannot use null in a heterogeneous array (arr or arr[any]) — cannot infer its concrete type.",
		Tip:         "Use a typed array like 'arr[int]' or replace null with a typed value.",
		Example:     "// Wrong:\narr x = [1, null, \"hi\"]\n\n// Right:\narr[int] x = [1, null, 2]",
	},
	"1543": {
		Code:        "1543",
		Message:     "Array of void",
		Description: "Cannot declare an array with element type 'void' — all elements would be null.",
		Tip:         "Use a concrete element type: arr[int], arr[string], etc.",
		Example:     "// Wrong:\narr[void] x = [null, null]\n\n// Right:\narr[int] x = [1, null, 2]",
	},
	"1544": {
		Code:        "1544",
		Message:     "Cannot infer array element type",
		Description: "The array element type could not be inferred from the elements.",
		Tip:         "Specify the element type explicitly: 'arr[int]', 'arr[string]', etc.",
	},
	"1545": {
		Code:        "1545",
		Message:     "Array element type mismatch (nested)",
		Description: "A nested array element's type doesn't match the declared element type.",
		Tip:         "Check the nested array's declared type.",
	},
	"1546": {
		Code:        "1546",
		Message:     "Cannot access field of non-error type",
		Description: "Field access (obj.field) requires the object to be an error type.",
		Tip:         "Only error instances have fields. Check the object's type.",
	},
	"1547": {
		Code:        "1547",
		Message:     "Field not found in error type",
		Description: "The error type has no field with this name.",
		Tip:         "Check the field name and the error type definition.",
	},
	"1548": {
		Code:        "1548",
		Message:     "Cannot throw non-error type",
		Description: "Only error instances can be thrown.",
		Tip:         "Use 'throw MyError{...}' where MyError is an error type.",
	},
	"1549": {
		Code:        "1549",
		Message:     "Unknown parent error type",
		Description: "The parent error type in a 'const' declaration does not exist.",
		Tip:         "Check the parent type name. Built-in parent is 'Error'.",
	},
	"1550": {
		Code:        "1550",
		Message:     "Field type mismatch with parent",
		Description: "A field's type doesn't match the parent's field type.",
		Tip:         "Use the same type as in the parent, or rename the field.",
	},
	"1551": {
		Code:        "1551",
		Message:     "Missing required field in error instance",
		Description: "An error instance is missing a field that has no default value.",
		Tip:         "Add the missing field, or give it a default in the error type.",
	},
	"1552": {
		Code:        "1552",
		Message:     "Cannot pass null to 'any' parameter",
		Description: "null cannot be passed to a parameter of type 'any' — cannot predict the future value type.",
		Tip:         "Use a concrete parameter type, or pass a typed null (e.g. via a typed variable).",
	},
	"1553": {
		Code:        "1553",
		Message:     "Cannot use null in arithmetic",
		Description: "null cannot be used in arithmetic operations.",
		Tip:         "Check the operands — they must be non-null numbers.",
	},
	"1554": {
		Code:        "1554",
		Message:     "Cannot use null in comparison",
		Description: "null cannot be used in comparison operations (except == and !=).",
		Tip:         "Check the operands.",
	},
	"1555": {
		Code:        "1555",
		Message:     "Cannot apply '!' to null",
		Description: "Logical NOT cannot be applied to null.",
		Tip:         "Check the operand — it must be a non-null bool.",
	},
	"1556": {
		Code:        "1556",
		Message:     "Function already used as error type name",
		Description: "A function and an error type cannot have the same name.",
		Tip:         "Rename one of them.",
	},
	"1557": {
		Code:        "1557",
		Message:     "Cannot assign null to 'any'",
		Description: "null cannot be assigned to a variable of type 'any' — cannot predict the future value type.",
		Tip:         "Use a concrete type, or leave the variable uninitialized.",
		Example:     "// Wrong:\nany x = null\n\n// Right:\nint x = null",
	},
	"1558": {
		Code:        "1558",
		Message:     "Cannot declare variable of type 'void'",
		Description: "A variable cannot have type 'void'.",
		Tip:         "Use a concrete type: int, string, arr[int], etc.",
		Example:     "// Wrong:\nvoid x = null\n\n// Right:\nint x = null",
	},
	"1559": {
		Code:        "1559",
		Message:     "Cannot assign null to 'void'",
		Description: "null cannot be assigned to a variable of type 'void'.",
		Tip:         "Use a concrete type.",
	},
	"1560": {
		Code:        "1560",
		Message:     "Unknown error type",
		Description: "The specified error type is not declared.",
		Tip:         "Declare the error type with 'const MyError{...} = new Error' before using it.",
	},

	// ============================================================================
	// 2000-2099: Warnings
	// ============================================================================
	"2000": {
		Code:        "2000",
		Message:     "Unused variable",
		Description: "The variable is declared but never used.",
		Tip:         "Remove the variable or use it.",
	},
	"2001": {
		Code:        "2001",
		Message:     "Unused parameter",
		Description: "The parameter is declared but never used.",
		Tip:         "Remove the parameter or use it.",
	},
	"2002": {
		Code:        "2002",
		Message:     "Untyped array",
		Description: "The array has no element type specified.",
		Tip:         "Use 'arr[int]' instead of 'arr'.",
	},
	"2003": {
		Code:        "2003",
		Message:     "Empty function body",
		Description: "The function has an empty body.",
		Tip:         "Add a comment or remove the function.",
	},
	"2004": {
		Code:    "2004",
		Message: "Could not check unused parameters",
		Description: "Function body contains an includeC block with raw C code. " +
			"Parameters used only inside that block cannot be detected by the compiler.",
		Tip: "If you know the parameters are used inside the includeC block — ignore this warning. " +
			"Otherwise remove unused parameters or use them in Skorpion code.",
	},

	// ============================================================================
	// 2500-2599: Backend / Codegen
	// ============================================================================
	"2500": {
		Code:        "2500",
		Message:     "IR generation error",
		Description: "Failed to generate intermediate representation.",
		Tip:         "This is an internal error. Please report it.",
	},

	// ============================================================================
	// 3000-3099: Builder / Linker
	// ============================================================================
	"3000": {
		Code:        "3000",
		Message:     "Compilation failed",
		Description: "The C compiler returned an error.",
		Tip:         "Check the C compiler output above.",
	},
	"3001": {
		Code:        "3001",
		Message:     "Compiler not found",
		Description: "No C compiler was found for the target OS.",
		Tip:         "Install gcc/clang or set a profile with 'skorpion add-win-profile'.",
	},
	"3002": {
		Code:        "3002",
		Message:     "Linker failed",
		Description: "The linker returned an error.",
		Tip:         "Check the linker output above.",
	},
	// ============================================================================
	// 3100-3199: Libraries
	// ============================================================================
	"3100": {
		Code:        "3100",
		Message:     "Cannot read library file",
		Description: "The .sklib file could not be read. It might be corrupted or have wrong permissions.",
		Tip:         "Re-copy the .sklib file, or rebuild it with 'skorpion build-lib'.",
	},
	"3101": {
		Code:        "3101",
		Message:     "Library not found",
		Description: "The library was not found in the configured libs directory.",
		Tip:         "Check the path and that libs[\"path/\"] in manifest.spc points to the right folder.",
		Example:     "libs/\n  author/\n    name/\n      LIBRARY-name-1.0.0.sklib",
	},
	"3102": {
		Code:        "3102",
		Message:     "Multiple library versions",
		Description: "Multiple .sklib files found in the library directory, but no @version was specified.",
		Tip:         "Specify the version: 'use lib:author/name@1.0.0'.",
	},
	"3103": {
		Code:        "3103",
		Message:     "Library version mismatch",
		Description: "The version in the import path doesn't match the version inside the .sklib file.",
		Tip:         "Check the version in the import path and the file name.",
	},
	"3104": {
		Code:        "3104",
		Message:     "Cannot write .sklib",
		Description: "Failed to write the library file. Check write permissions.",
		Tip:         "Check write permissions for the output directory.",
	},
	"3105": {
		Code:        "3105",
		Message:     "Library has no exportable functions",
		Description: "The library project has no functions marked as exportable.",
		Tip:         "Remove '*' from function signatures to make them exportable.",
		Example:     "// Non-exportable (has '*'):\nint* helper() { ... }\n\n// Exportable:\nint helper() { ... }",
	},
	"3106": {
		Code:        "3106",
		Message:     "Invalid library spec",
		Description: "The library path is malformed. Expected format: 'lib:author/name[@version]'.",
		Tip:         "Use: 'use lib:author/name@1.0.0'.",
		Example:     "// Wrong:\nuse lib:name\nuse lib:author\n\n// Right:\nuse lib:author/name@1.0.0",
	},
	"3107": {
		Code:        "3107",
		Message:     "Library dependency not imported",
		Description: "The library requires a dependency that is not imported in the current project.",
		Tip:         "Add 'use lib:...' for the dependency BEFORE the library that needs it.",
		Example:     "// Right order:\nuse lib:author/b@1.0.0\nuse lib:author/a@1.0.0  // a depends on b",
	},
	"3108": {
		Code:        "3108",
		Message:     "Library has no authors",
		Description: "A library must declare at least one author in manifest.spc. The first author is used as the directory name in libs/.",
		Tip:         "Add authors[(\"your-name\")] to manifest.spc.",
		Example:     "authors[(\"iamtowvee\")]",
	},
	"3109": {
		Code:        "3109",
		Message:     "Library author mismatch",
		Description: "The author in the import path doesn't match the author declared inside the .sklib file.",
		Tip:         "Check that the library is placed in the correct author directory.",
		Example:     "// If library was built by 'iamtowvee':\nlibs/iamtowvee/name/LIBRARY-name-1.0.0.sklib",
	},
}
