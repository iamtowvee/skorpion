package errors

type ErrorCode struct {
	Code        string
	Message     string
	Description string
	Tip         string
}

var ErrorCodes = map[string]ErrorCode{
	"1043": {
		Code:        "1043",
		Message:     "Type mismatch",
		Description: "The type of the value you provided doesn't match the expected type.",
		Tip:         "Check the function signature and make sure you're passing the correct type. Use $ to convert int to string, or ref.toInt() to convert string to int.",
	},
	"1001": {
		Code:        "1001",
		Message:     "File not found",
		Description: "The specified file could not be found.",
		Tip:         "Check that the file path is correct and the file exists.",
	},
	"1004": {
		Code:        "1004",
		Message:     "No main() function found",
		Description: "Every Skorpion program needs a main() function as entry point.",
		Tip:         "Add 'void main(arr args) { ... }' to your program.",
	},
	// ... все остальные коды
}
