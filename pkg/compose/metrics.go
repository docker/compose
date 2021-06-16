/*
   Copyright 2020 Docker Compose CLI authors

   Licensed under the Apache License, Version 2.0 (the "License");
   you may not use this file except in compliance with the License.
   You may obtain a copy of the License at

       http://www.apache.org/licenses/LICENSE-2.0

   Unless required by applicable law or agreed to in writing, software
   distributed under the License is distributed on an "AS IS" BASIS,
   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
   See the License for the specific language governing permissions and
   limitations under the License.
*/

package compose

// FailureCategory sruct regrouping metrics failure status and specific exit code
type FailureCategory struct {
	MetricsStatus string
	ExitCode      int
}

const (
	// APISource is sent for API metrics
	APISource = "api"
	// SuccessStatus command success
	SuccessStatus = "success"
	// FailureStatus command failure
	FailureStatus = "failure"
	// ComposeParseFailureStatus failure while parsing compose file
	ComposeParseFailureStatus = "failure-compose-parse"
	// FileNotFoundFailureStatus failure getting compose file
	FileNotFoundFailureStatus = "failure-file-not-found"
	// CommandSyntaxFailureStatus failure reading command
	CommandSyntaxFailureStatus = "failure-cmd-syntax"
	// BuildFailureStatus failure building imge
	BuildFailureStatus = "failure-build"
	// PullFailureStatus failure pulling imge
	PullFailureStatus = "failure-pull"
	// CanceledStatus command canceled
	CanceledStatus = "canceled"
)

var (
	// FileNotFoundFailure failure for compose file not found
	FileNotFoundFailure = FailureCategory{MetricsStatus: FileNotFoundFailureStatus, ExitCode: 14}
	// ComposeParseFailure failure for composefile parse error
	ComposeParseFailure = FailureCategory{MetricsStatus: ComposeParseFailureStatus, ExitCode: 15}
	// CommandSyntaxFailure failure for command line syntax
	CommandSyntaxFailure = FailureCategory{MetricsStatus: CommandSyntaxFailureStatus, ExitCode: 16}
	//BuildFailure failure while building images.
	BuildFailure = FailureCategory{MetricsStatus: BuildFailureStatus, ExitCode: 17}
	// PullFailure failure while pulling image
	PullFailure = FailureCategory{MetricsStatus: PullFailureStatus, ExitCode: 18}
)

//ByExitCode retrieve FailureCategory based on command exit code
func ByExitCode(exitCode int) FailureCategory {
	switch exitCode {
	case 0:
		return FailureCategory{MetricsStatus: SuccessStatus, ExitCode: 0}
	case 14:
		return FileNotFoundFailure
	case 15:
		return ComposeParseFailure
	case 16:
		return CommandSyntaxFailure
	case 17:
		return BuildFailure
	case 18:
		return PullFailure
	case 130:
		return FailureCategory{MetricsStatus: CanceledStatus, ExitCode: exitCode}
	default:
		return FailureCategory{MetricsStatus: FailureStatus, ExitCode: exitCode}
	}
}
