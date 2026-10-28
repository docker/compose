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

// Package api is the contract between the Compose engine and the programs
// that drive it: the compose CLI, and any third-party software using Compose
// as a library (see [Compose]).
//
// # Zero values
//
// An option struct left at its zero value does NOT always behave like the
// matching `docker compose` command run without flags: the CLI fills in some
// defaults before calling the API. The fields where the difference matters
// are listed here; each one is also documented on the field itself.
//
//   - [CreateOptions].Inherit: false recreates the anonymous volumes of
//     recreated containers, which is what `up --renew-anon-volumes` does. The
//     CLI default is to retrieve their data from the previous container
//     (Inherit: true).
//   - [CreateOptions].Recreate and RecreateDependencies: the empty string
//     means [RecreateDiverged], and so does any value that is not one of the
//     Recreate* constants.
//   - [CreateOptions].RemoveOrphans and IgnoreOrphans: when both are false,
//     orphan containers are kept and a warning is logged. IgnoreOrphans
//     silences that warning, RemoveOrphans removes the containers.
//   - Timeout, on every options struct that has one: nil leaves the choice to
//     the engine, which applies the stop timeout of the container (the
//     service's stop_grace_period) or its own default.
//   - [StartOptions].WaitTimeout: 0 means no time limit on dependency waits.
//   - [StartOptions].Attach: nil selects the detached mode of Up, a non-nil
//     consumer selects the interactive session. See [UpOptions.Validate] for
//     the options that only make sense in one of the two modes.
//   - Project, on the options of operations that take a project name: for
//     Start, Stop, Restart, Kill and Down, nil means the project is rebuilt
//     from the labels of its containers. Pause, Remove and Logs only use it
//     to narrow the services, and Ps and Attach ignore it.
//   - Services, when empty: every service of the project, subject to what the
//     field documents (in some options it is not a filter).
//   - [DownOptions].Images: the zero value, [ImagePruneNone], keeps every image.
//
// # Replica selection
//
// The Index field of the options of exec, port, export, commit, attach, logs
// and cp selects a replica by its container number, starting at 1. Index 0
// means "no particular replica", and what that gives depends on the
// operation:
//
//   - exec, port, export, commit and attach act on one container: the one
//     with the lowest number, one-off containers coming last;
//   - logs reads the containers of every selected service, one-off containers
//     excluded;
//   - cp copies from the first container of the service, or to every
//     container of the service when copying to it. One-off containers are
//     excluded unless All is set, and All only applies when Index is 0: a
//     given replica is looked up among the regular containers.
//
// # The word All
//
// The boolean All field has a different meaning in each options struct, and
// is always documented there:
//
//   - [ListOptions]: takes stopped containers into account, so that a
//     project whose containers are all stopped is listed;
//   - [PsOptions]: includes the stopped containers and the one-off ones;
//   - [KillOptions]: also targets the containers that are not running (the
//     one-off ones are targeted either way);
//   - [CopyOptions]: includes the one-off containers of the service;
//   - [ProjectLoadOptions]: includes the resources no service uses.
package api
