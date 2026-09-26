package errors

type ErrorCode struct {
	Code        string
	Message     string
	Description string
	Tip         string
}

var ErrorCodes = map[string]ErrorCode{
	// ============ 0000: Unknown ============
	"0000": {
		Code:        "0000",
		Message:     "Unknown error",
		Description: "An unknown error occurred.",
		Tip:         "Please report this bug with the full output.",
	},

	// ============ 0010-0099: Compiler / Internal ============
	"0010": {
		Code:        "0010",
		Message:     "Cannot read config file",
		Description: "The manifest.spc file could not be read.",
		Tip:         "Check that manifest.spc exists and is readable.",
	},
	"0011": {
		Code:        "0011",
		Message:     "File not found",
		Description: "The specified file could not be found.",
		Tip:         "Check that the file path is correct and the file exists.",
	},
	"0020": {
		Code:        "0020",
		Message:     "Cannot write C file",
		Description: "Failed to write the generated C code to disk.",
		Tip:         "Check write permissions for the output directory.",
	},
	"0021": {
		Code:        "0021",
		Message:     "Compilation failed",
		Description: "The C compiler returned an error.",
		Tip:         "Check the C compiler output above for details.",
	},
	"0022": {
		Code:        "0022",
		Message:     "Cannot create output directory",
		Description: "Failed to create the build output directory.",
		Tip:         "Check permissions and the output path in manifest.spc.",
	},

	// ============ 0100-0499: Lexer ============
	"0100": {
		Code:        "0100",
		Message:     "Unknown character",
		Description: "The lexer encountered a character it doesn't recognize.",
		Tip:         "Check for typos or invalid symbols in your code.",
	},
	"0101": {
		Code:        "0101",
		Message:     "Unknown character '|'",
		Description: "A single '|' is not valid. Did you mean '||'?",
		Tip:         "Use '||' for logical OR.",
	},
	"0102": {
		Code:        "0102",
		Message:     "Unterminated string",
		Description: "A string literal was not closed with a closing quote.",
		Tip:         "Add a closing '\"' to the string.",
	},
	"0103": {
		Code:        "0103",
		Message:     "Unterminated backticks",
		Description: "A ``` block (includeC) was not closed.",
		Tip:         "Add closing ``` after the C code.",
	},
	"0104": {
		Code:        "0104",
		Message:     "Unterminated multi-line comment",
		Description: "A /* comment was not closed with */.",
		Tip:         "Add '*/' to close the comment.",
	},

	// ============ 0500-1499: Parser ============
	"0500": {
		Code:        "0500",
		Message:     "Unexpected token",
		Description: "The parser found an unexpected token.",
		Tip:         "Check the syntax around this location.",
	},
	"0501": {
		Code:        "0501",
		Message:     "Expected function name",
		Description: "After the return type, a function name is expected.",
		Tip:         "Add a function name: 'void myFunc() { ... }'.",
	},
	"0502": {
		Code:        "0502",
		Message:     "Expected parameter type",
		Description: "A parameter must start with a type.",
		Tip:         "Use: 'void func(int x, string s) { ... }'.",
	},
	"0503": {
		Code:        "0503",
		Message:     "Expected parameter name",
		Description: "After the parameter type, a name is expected.",
		Tip:         "Use: 'void func(int x) { ... }'.",
	},
	"0504": {
		Code:        "0504",
		Message:     "Expected variable name",
		Description: "After the variable type, a name is expected.",
		Tip:         "Use: 'int x = 10'.",
	},
	"0505": {
		Code:        "0505",
		Message:     "Unexpected keyword in expression",
		Description: "A keyword appeared where an expression was expected.",
		Tip:         "Check the expression syntax.",
	},
	"0506": {
		Code:        "0506",
		Message:     "Unexpected token in expression",
		Description: "The parser found an unexpected token in an expression.",
		Tip:         "Check the expression syntax.",
	},
	"0507": {
		Code:        "0507",
		Message:     "Expected '('",
		Description: "An opening parenthesis was expected.",
		Tip:         "Add '(' at this location.",
	},
	"0508": {
		Code:        "0508",
		Message:     "Expected ')'",
		Description: "A closing parenthesis was expected.",
		Tip:         "Add ')' at this location.",
	},
	"0509": {
		Code:        "0509",
		Message:     "Expected '{'",
		Description: "An opening brace was expected.",
		Tip:         "Add '{' at this location.",
	},
	"0510": {
		Code:        "0510",
		Message:     "Expected ``` after includeC",
		Description: "includeC must be followed by ``` with C code.",
		Tip:         "Use: includeC ``` ... ```.",
	},
	"0511": {
		Code:        "0511",
		Message:     "Expected function call after '.'",
		Description: "After '.', a function name and '(' are expected.",
		Tip:         "Use: 'module.func()'.",
	},
	"0512": {
		Code:        "0512",
		Message:     "Expected ',' after case branch",
		Description: "Each case branch must end with a comma.",
		Tip:         "Add ',' after the branch body.",
	},
	"0513": {
		Code:        "0513",
		Message:     "Expected type in arr[]",
		Description: "arr[] requires an element type.",
		Tip:         "Use: 'arr[int]', 'arr[string]', etc.",
	},
	"0514": {
		Code:        "0514",
		Message:     "Expected ']' in array type",
		Description: "The array type was not closed.",
		Tip:         "Add ']' after the element type.",
	},
	"0515": {
		Code:        "0515",
		Message:     "Expected ']'",
		Description: "A closing bracket was expected.",
		Tip:         "Add ']' at this location.",
	},
	"0516": {
		Code:        "0516",
		Message:     "Expected ']' in array index",
		Description: "The array index was not closed.",
		Tip:         "Add ']' after the index expression.",
	},
	"0520": {
		Code:        "0520",
		Message:     "Expected method name after '.'",
		Description: "After '.', a method name is expected.",
		Tip:         "Use: 'obj.method()'.",
	},
	"0521": {
		Code:        "0521",
		Message:     "Expected ':' in ternary",
		Description: "A ternary expression requires ':' between branches.",
		Tip:         "Use: 'cond ? a : b'.",
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
		Description: "The parser reached the end of the file unexpectedly.",
		Tip:         "Check for missing closing braces or brackets.",
	},

	// ============ 1500-1999: Semantic Errors ============
	"1500": {
		Code:        "1500",
		Message:     "Function already declared",
		Description: "A function with this name already exists.",
		Tip:         "Rename the function or remove the duplicate.",
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
		Tip:         "Valid types: int, string, float, double, bool, char, arr, dict, any.",
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

	// ============ 2000-2499: Semantic Warnings ============
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

	// ============ 2500-2999: Backend / Codegen ============
	"2500": {
		Code:        "2500",
		Message:     "IR generation error",
		Description: "Failed to generate intermediate representation.",
		Tip:         "This is an internal error. Please report it.",
	},

	// ============ 3000-3499: Builder / Linker ============
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
}
