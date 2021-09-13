
## Description

Services are built once and then tagged, by default as `project_service`. 

If the Compose file specifies an
[image](https://github.com/compose-spec/compose-spec/blob/master/spec.md#image) name, 
the image is tagged with that name, substituting any variables beforehand. See
[variable interpolation](https://github.com/compose-spec/compose-spec/blob/master/spec.md#interpolation).

If you change a service's `Dockerfile` or the contents of its build directory, 
run `docker compose build` to rebuild it.
