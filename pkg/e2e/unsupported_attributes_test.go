//go:build e2e

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

package e2e

import "testing"

// TestUnsupportedAttributesWarning locks docker/compose#13150: Compose must
// warn, at load time, when a compose-file attribute is accepted by the
// schema but has no effect on this runtime outside Swarm mode, instead of
// silently ignoring it. It also locks two false positives this check must
// NOT produce: a plain published port (the loader defaults ports[].mode to
// "ingress" for every port entry), and label_file (compose-go's loader
// resolves it into real service labels by default — it's fully functional,
// unlike the genuinely Swarm-only attributes here). configs[].driver_opts
// has no effect and no warning either, since the compose-spec schema has no
// such property on a top-level configs entry (unlike secrets[].driver_opts,
// which is schema-valid and is flagged).
func TestUnsupportedAttributesWarning(t *testing.T) {
	// logrus's text formatter escapes embedded quotes in a field's value, so
	// the raw output contains a literal backslash before each quote around
	// the service name (`msg="service \"x\": ..."`) — don't "clean up" these
	// backslashes, the checks stop matching if you do.
	NewScenario(t, "compose must warn about schema-valid attributes it doesn't honor outside Swarm mode, and stay silent on the ones it does honor").
		Step("every schema-valid, silently-ignored attribute is reported, and honored/unreachable attributes are not",
			ComposeCmd("config"),
			OutputContains(`service \"swarm-attrs\": deploy.mode`),
			OutputContains(`service \"swarm-attrs\": deploy.labels`),
			OutputContains(`service \"swarm-attrs\": deploy.update_config`),
			OutputContains(`service \"swarm-attrs\": deploy.rollback_config`),
			OutputContains(`service \"swarm-attrs\": deploy.placement`),
			OutputContains(`service \"swarm-attrs\": deploy.endpoint_mode`),
			OutputContains(`service \"swarm-attrs\": credential_spec`),
			OutputContains(`service \"ports-demo\": ports[80/tcp].mode`),
			OutputContains(`service \"cluster-vol\": volumes[my-csi-volume].type`),
			OutputContains(`service \"file-refs\": configs.myconfig.uid`),
			OutputContains(`service \"file-refs\": configs.myconfig.gid`),
			OutputContains(`service \"file-refs\": configs.myconfig.mode`),
			OutputContains(`service \"file-refs\": secrets.mysecret.uid`),
			OutputContains(`service \"file-refs\": secrets.mysecret.gid`),
			OutputContains(`service \"file-refs\": secrets.mysecret.mode`),
			OutputContains(`configs.myconfig.labels`),
			OutputContains(`secrets.mysecret.driver_opts`),
			OutputContains(`secrets.mysecret.labels`),
			OutputNotContains(`service \"clean\"`),
			OutputNotContains(`configs.myconfig.driver_opts`))
}
