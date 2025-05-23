# docker compose bridge convert

<!---MARKER_GEN_START-->
Convert compose files to Kubernetes manifests, Helm charts, or another model

### Options

| Name                     | Type          | Default | Description                                                                          |
|:-------------------------|:--------------|:--------|:-------------------------------------------------------------------------------------|
| `--dry-run`              | `bool`        |         | Execute command in dry run mode                                                      |
| `-o`, `--output`         | `string`      | `out`   | The output directory for the Kubernetes resources                                    |
| `--templates`            | `string`      |         | Directory containing transformation templates                                        |
| `-t`, `--transformation` | `stringArray` |         | Transformation to apply to compose model (default: docker/compose-bridge-kubernetes) |


<!---MARKER_GEN_END-->

