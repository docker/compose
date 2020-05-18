node_modules/.bin/grpc_tools_node_protoc \
    --js_out=import_style=commonjs,binary:./grpc \
    --grpc_out=generate_package_definition:./grpc \
    -I ../../cli/v1 \
    -I ../../containers/v1 \
    ../../cli/v1/*.proto \
    ../../containers/v1/*.proto

# generate d.ts codes
protoc \
    --plugin=protoc-gen-ts=./node_modules/.bin/protoc-gen-ts \
    --ts_out=generate_package_definition:./grpc \
    -I ../../cli/v1 \
    -I ../../containers/v1 \
    ../../cli/v1/*.proto \
    ../../containers/v1/*.proto
