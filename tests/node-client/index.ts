import * as grpc from "@grpc/grpc-js";
import { ContainersClient } from "./grpc/containers_grpc_pb";
import { ContextsClient } from "./grpc/contexts_grpc_pb";
import { ListRequest, ListResponse } from "./grpc/containers_pb";
import { SetCurrentRequest } from "./grpc/contexts_pb";

let address = process.argv[3] || "unix:///tmp/backend.sock";

const client = new ContainersClient(address, grpc.credentials.createInsecure());
const contextsClient = new ContextsClient(
  address,
  grpc.credentials.createInsecure()
);

let backend = process.argv[2] || "moby";

contextsClient.setCurrent(new SetCurrentRequest().setName(backend), () => {
  client.list(new ListRequest(), (err: any, response: ListResponse) => {
    if (err != null) {
      console.error(err);
      return;
    }

    const containers = response.getContainersList();

    containers.forEach((container) => {
      console.log(container.getId(), container.getImage());
    });
  });
});
