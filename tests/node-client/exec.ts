import * as grpc from "@grpc/grpc-js";
import * as readline from "readline";
import * as google_protobuf_any_pb from "google-protobuf/google/protobuf/any_pb.js";

import * as continersPb from "./grpc/containers_grpc_pb";
import { IContainersClient } from "./grpc/containers_grpc_pb";
import { ExecRequest, ExecResponse, LogsRequest } from "./grpc/containers_pb";

import * as streamsPb from "./grpc/streams_grpc_pb";
import { IStreamingClient } from "./grpc/streams_grpc_pb";
import { BytesMessage } from "./grpc/streams_pb";

let address = process.argv[3] || "unix:///tmp/backend.sock";

const ContainersServiceClient = grpc.makeClientConstructor(
  continersPb["com.docker.api.protos.containers.v1.Containers"],
  "ContainersClient"
);

const client = (new ContainersServiceClient(
  address,
  grpc.credentials.createInsecure()
) as unknown) as IContainersClient;

const StreamsServiceClient = grpc.makeClientConstructor(
  streamsPb["com.docker.api.protos.streams.v1.Streaming"],
  "StreamsClient"
);

let streamClient = (new StreamsServiceClient(
  address,
  grpc.credentials.createInsecure()
) as unknown) as IStreamingClient;

let backend = process.argv[2] || "moby";
let containerId = process.argv[3];
const meta = new grpc.Metadata();
meta.set("CONTEXT_KEY", backend);

// Get the stream
const stream = streamClient.newStream();

stream.on("metadata", (m: grpc.Metadata) => {
  let req = new ExecRequest();
  req.setCommand("/bin/bash");
  req.setStreamId(m.get("id")[0] as string);
  req.setId(containerId);
  req.setTty(true);

  client.exec(req, meta, (err: any, _: ExecResponse) => {
    if (err != null) {
      console.error(err);
      return;
    }
    process.exit();
  });
});

readline.emitKeypressEvents(process.stdin);
process.stdin.setRawMode(true);

process.stdin.on("keypress", (str, key) => {
  const mess = new BytesMessage();
  const a = new Uint8Array(key.sequence.length);
  for (let i = 0; i <= key.sequence.length; i++) {
    a[i] = key.sequence.charCodeAt(i);
  }

  mess.setValue(a);

  const any = new google_protobuf_any_pb.Any();
  any.pack(
    mess.serializeBinary(),
    "type.googleapis.com/com.docker.api.protos.streams.v1.BytesMessage"
  );
  stream.write(any);
});

stream.on("data", (chunk: any) => {
  const m = chunk.unpack(
    BytesMessage.deserializeBinary,
    "com.docker.api.protos.streams.v1.BytesMessage"
  ) as BytesMessage;
  process.stdout.write(m.getValue());
});
