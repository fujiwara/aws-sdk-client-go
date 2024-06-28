# awslim

`awslim` is a CLI for AWS services by [Go](https://go.dev/). This CLI is generated from the [AWS SDK Go v2](https://github.com/aws/aws-sdk-go-v2) service client.

(The old name is `aws-sdk-client-go`.)

## Motivation

While the [AWS CLI](https://aws.amazon.com/cli/) is very useful, it can be resource intensive to boot up. `awslim` offers a simpler and faster alternative for limited use cases. It acts as a simple wrapper around AWS SDK Go v2, providing essential functionality without the full feature set of the AWS CLI.


### Features

- Call any method(API) of the AWS service client.
- Use JSON or Jsonnet for input.
- Output the result in JSON format.
- Bind a file to the input/output struct of the method.
- Query the output by JMESPath.
- Use the AWS CLI configuration file. (i.e., `~/.aws/config`)

### Limitations

- Not 100% compatible with the AWS CLI.
- No support for AWS CLI plugins. (i.e., `session-manager-plugin`)
- Some service names and different from the AWS CLI. (i.e., `logs` -> `cloudwatchlogs`, `ce` -> `costexplorer`)

## Installation

**Note:** The release binaries are large (about 500MB after being extracted) and have a slight startup delay (about 100ms), due to the inclusion of code to access **[all AWS services](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service)**. However, it is still [faster](#performance-comparison) than the AWS CLI.

For optimized performance, build your own binary tailored to the specific services you need. See the [Build](#build) section for details.

### Release binary

Download the binary from the [release page](https://github.com/fujiwara/awslim/releases).

### Homebrew

```console
$ brew install fujiwara/tap/awslim
```

## Build

You can build the client yourself, including only the needed services and methods. This produces a smaller, faster binary.

The client is built by using a `AWSLIM_GEN` environment variable or a `gen.yaml` configuration file.

### `AWSLIM_GEN` environment variable

Set the `AWSLIM_GEN` environment variable to specify the services you want to include, separated by commas.

For example, to build the client for ECS, Firehose, and S3:

```console
$ export AWSLIM_GEN="ecs,firehose,s3"
```

This will generate all methods for the specified services. If you want to generate only specific methods, use the gen.yaml configuration file.

### `gen.yaml` configuration file

```yaml
# gen.yaml
services:
  ecs:
    - DescribeClusters
    - DescribeTasks
  firehose:
    - DescribeDeliveryStream
    - ListDeliveryStreams
  s3:
    # all methods of the service
```

Keys under `services` are AWS service names (`github.com/aws/aws-sdk-go-v2/service/*`), and values are method names of the service client (for example, `s3` is [s3.Client](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3#Client)). If you don't specify the method names, all methods of the service client are generated.

### Build binary for specified OS/Architecture

Set the `AWSLIM_OS` and `AWSLIM_ARCH` environment variables to specify the OS and architecture for the build.
If these variables are not set, awslim will build based on the architecture of the build environment.
These variables can also be specified when building with Docker.
The values that can be set for AWSLIM_OS and AWSLIM_ARCH are the same as for GOOS and GOARCH.

### Build on your machine

To build the client, run the following commands (or simply run `make`):

```console
$ go generate ./cmd/awslim-gen .
$ go build -o your-client ./cmd/awslim/main.go
```

1. `go generate ./cmd/awslim-gen .` generates the generator by `gen.yaml`.
2. `go build -o your-client ./cmd/awslim/main.go` builds your client.

If you change the configuration, run `make clean` before `make` to purge the generated files.

### Build with Docker

Use the `ghcr.io/fujiwara/awslim:builder` builder image to build the client inside a container.

Environment variables:
- `GIT_REF`: Git reference to checkout the repository. Default is `main`. You can specify a branch, tag, or commit hash.

Example using the `AWSLIM_GEN` environment variable:
```console
$ docker run -it -e AWSLIM_GEN=ecs,firehose,s3 ghcr.io/fujiwara/awslim:builder
...
```

Example to specify build for OS and Architecture:

```console
$ docker run -it -e AWSLIM_GEN=ecs,firehose,s3 -e AWSLIM_OS=linux -e AWSLIM_ARCH=arm64 ghcr.io/fujiwara/awslim:builder
...
```

Example using the `gen.yaml` configuration file:
```console
$ docker run -it -v $(pwd)/gen.yaml:/app/gen.yaml ghcr.io/fujiwara/awslim:builder
...
Completed. Please extract /app/awslim from this container!
For example, run the following command:
docker cp $(docker ps -lq):/app/awslim .
```

After the build is complete, copy the binary to your host machine with the `docker cp` command.

```console
$ docker cp $(docker ps -lq):/app/awslim .
```

### Docker Multi-stage build

It is also possible to use the `ghcr.io/fujiwara/awslim:builder` builder image in a multi-stage build.

Run `./build-in-docker.sh` in the container to build the client. The built binary will be located in the `/app` directory. You can then copy it to the final image.

```Dockerfile
FROM ghcr.io/fujiwara/awslim:builder AS builder
ENV AWSLIM_GEN=ecs,firehose,s3
ENV GIT_REF=v0.1.2
RUN ./build-in-docker.sh

FROM debian:bookworm-slim
COPY --from=builder /app/awslim /usr/local/bin/awslim
```

## Performance comparison

Example of executing `sts get-caller-identity` on a 0.25 vCPU Fargate(AMD64) using `/usr/bin/time -v` for time measurement.

| command | CPU time(user, sys)| Elapsed time(s) | Max memory(MB) | Size(MB) |
| ---- | ---- | ---- | --- | --- |
| aws         | 0.67 + 0.10 = 0.77 | 3.11 | 64.2  | 225 |
| awslim(all) | 0.08 + 0.03 = 0.11 | 0.43 | 101.5 | 476 |
| awslim(40)  | 0.02 + 0.01 = 0.03 | 0.05 | 30.2  |  95 |

- `awslim`(built for all AWS services): 7.0x faster than `aws`
- `awslim`(built for 40 AWS services): 62.0x faster than `aws`

`aws-cli/2.15.51 Python/3.11.8`, `awslim 0.1.0`

## Usage

```
Usage: awslim [<service> [<method> [<input>]]] [flags]

Arguments:
  [<service>]    service name
  [<method>]     method name
  [<input>]      input JSON/Jsonnet struct or filename

Flags:
  -h, --help                      Show context-sensitive help.
  -i, --input-stream=STRING       bind input filename or '-' to io.Reader field in the input struct
  -o, --output-stream=STRING      bind output filename or '-' to io.ReadCloser field in the output struct
      --[no-]api-output           output API response into stdout
  -r, --raw-output                output raw strings, not JSON texts
  -c, --compact                   compact JSON output
  -q, --query=STRING              JMESPath query to apply to output
      --ext-str=KEY=VALUE;...     external variables for Jsonnet
      --ext-code=KEY=VALUE;...    external code for Jsonnet
      --[no-]strict               strict input JSON unmarshaling
  -f, --follow-next=""            OutputField=InputField format. follow the next token.
      --camel                     convert keys to camelCase
  -n, --dry-run                   dry-run mode
  -v, --version                   show version
```

- `service`: AWS service name.
- `method`: Method name of the service client.
- `input`: JSON input for the method.

The output is JSON format.

### Example usage

#### Show supported services

```console
$ awslim
```

#### Show methods of the service

```console
$ awslim ecs
```

#### Call method of the service

The third argument is a [JSON](https://json.org) or [Jsonnet](https://jsonnet.org/) input for the method. This can be omitted if the method requires no input (`{}` is passed implicitly).

```console
$ awslim ecs DescribeClusters '{"Cluster":"default"}' # JSON
```

```console
$ awslim ecs DescribeClusters "{Cluster:'default'}"   # Jsonnet
```

If the method name is "kebab-case", it automatically converts to "PascalCase" (i.e., `describe-clusters` -> `DescribeClusters`).

```console
$ awslim ecs describe-clusters '{"Cluster":"default"}'
```

The third argument can be a filename that contains JSON or Jsonnet input.

```console
$ awslim ecs DescribeClusters my.jsonnet
```

**Note**: By default, the input JSON is unmarshaled strictly. Unknown fields for the input struct in the input JSON cause an error. If you want to unmarshal the input JSON non-strictly, use `--no-strict` option.

#### Jsonnet functions

awslim supports Jsonnet functions for the input JSON.

```console
$ awslim ecs describe-clusters '{Cluster: _(0)}' foo

$ CLUSTER=foo awslim ecs describe-clusters '{Cluster: env("CLUSTER","default")}'

$ CLUSTER=foo awslim ecs describe-clusters '{Cluster: must_env("CLUSTER")}'
```

- `_(n)` returns the n-th argument.
- `env(n, d)` returns the environment variable `n`. If the environment variable is not set, it returns `d`.
- `must_env(n)` returns the environment variable `n`. If the environment variable is not set, it causes an error.

When you specify the input JSON as a file, you must define the function in the Jsonnet file.

```jsonnet
local _ = std.native('args');
local env = std.native('env');
local must_env = std.native('must_env');
{
  Cluster: _(0),
}
```

#### `--ext-str` and `--ext-code` options

Pass external variables to Jsonnet.

This is useful when you want to use variables in Jsonnet.

```console
$ awslim ecs DescribeClusters my.jsonnet --ext-str Cluster=default
```

```jsonnet
// my.jsonnet
{
  Cluster: std.extVar("Cluster"),
}
```

#### `--input-stream (-i)` option

Bind a file or stdin to the input struct.

```console
$ awslim s3 put-object '{"Bucket": "my-bucket", "Key": "my.txt"}' --input-stream my.txt
```

[s3#PutObjectInput](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3#PutObjectInput) has `Body` field of `io.Reader`. `--input-stream` option binds the file to the field.

When the input struct has only one field of `io.Reader`, `awslim` reads the file and binds it to the field automatically. (Currently, all SDK input structs have at most one io.Reader field.)

When the input struct has a "\*Length" field for the size of the content, `awslim` sets the size of the content to the field automatically. For example, [s3#PutObjectInput](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3#PutObjectInput) has `ContentLength` field.

If `--input-stream` is "-", `awslim` reads from stdin. In this case, `awslim` reads all contents into memory, so it is not suitable for large files. Consider using a file for large content.

#### `--output-stream (-o)` option

Bind the `io.ReadCloser` of the API output to a file or stdout.

```console
$ awslim s3 get-object '{"Bucket": "my-bucket", "Key": "my.txt"}' --output-stream my.txt
```

[s3#GetObjectOutput](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3#PutObjectOutput) has `Body` field of `io.ReadeCloser`. `--output-stream` option binds the file to the field.

When the output struct has only one field of `io.ReadCloser`, `awslim` copies it to the file automatically. (Currently, all SDK output structs have at most one io.ReadCloser field.)

If `--output-stream` is "-", `awslim` writes into stdout. The result of the API also writes to stdout by default. If you don't want to output the result, use `--no-api-output`.

#### `--raw-output (-r)` option

Output raw strings, not JSON texts.

This option is like `jq -r`.

#### `--follow-next (-f)` option

Set the output/input field name of the next token. This option is useful for paginated APIs.

For example, [s3#ListObjectsV2Output](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3#ListObjectsV2Output) has a `NextContinuationToken` field, and [s3#ListObjectsV2Input](https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/s3#ListObjectsV2Input) has a `ContinuationToken` field. You can follow the next token by the following command.

`{FieldInOutput}={FieldInInput}` format is used for `--follow-next` option.

```console
$ awslim s3 list-objects-v2 '{"Bucket": "my-bucket"}' \
  --follow-next NextContinuationToken=ContinuationToken
```

If the same field name is used in the output and input, you can omit the input field name.

```console
$ awslim ecs list-tasks '{"Cluster":"default"}' \
  --follow-next NextToken
```

#### `--camel` option

Convert JSON keys to camelCase.

By default, the output JSON keys are the same as the SDK struct field names (equivalent to PascalCase).

In the JSON output produced by AWS CLI, the key naming conventions are either PascalCase or camelCase, determined by the service. For example, `aws ecs` uses camelCase, while `aws lambda` uses PascalCase.

If you want to convert the keys to camelCase, use the `--camel` option.

```console
$ awslim ecs describe-clusters '{Clusters:["default"]}' --camel
{
  "clusters": [
    {
      "activeServicesCount": 1,
      "capacityProviders": [],
      "clusterArn": "arn:aws:ecs:ap-northeast-1:123456789012:cluster/default",
      "clusterName": "default",
      "defaultCapacityProviderStrategy": [],
      "pendingTasksCount": 0,
      "registeredContainerInstancesCount": 0,
      "runningTasksCount": 0,
      "settings": [],
      "statistics": [],
      "status": "ACTIVE",
      "tags": []
    }
  ],
  "failures": [],
  "resultMetadata": {}
}
```

This conversion is performed mechanically, so objects for which any key can be specified (such as the dockerLabels element in an ECS task definition) are also subject to conversion.

It is not guaranteed that the results will match those in the AWS CLI output.

#### Query output by JMESPath

Query the output by JMESPath like the AWS CLI.

```console
$ awslim ecs DescribeClusters '{"Cluster":"default"}' \
  --query 'Clusters[0].ClusterArn'
```

#### Show help

For method-specific documentation, use the `help` argument to display the URL of the method's documentation. Since `awslim` is a simple wrapper for the AWS SDK Go v2 service client, its usage mirros that of the SDK.

```console
$ awslim ecs DescribeClusters help
See https://pkg.go.dev/github.com/aws/aws-sdk-go-v2/service/ecs#Client.DescribeClusters
```

### Runtime Configurations

`awslim` reads the configuration file (`~/.config/awslim/config.(json|jsonnet|yaml|yml)`). The configuration file can contain the following fields:

```yaml
open: /usr/bin/open
aliases:
  whoami: sts get-caller-identity
  regions: ec2 describe-regions --query Regions[].RegionName
```

- `open`: The command to open the URL to the documentation.
- `aliases`: The alias of the cli arguments.
  For example, `awslim whoami` is equivalent to `awslim sts get-caller-identity`.

`XDG_CONFIG_HOME` environment variable is used for the configuration file path. If `XDG_CONFIG_HOME` is not set, `~/.config` is used.

## LICENSE

MIT

## Author

Fujiwara Shunichiro
