# SSM Shell Example
An example program for creating an SSM Shell session.

## Build
`go build`  
This will output a file named `ssm-shell` in the current directory.

## Usage
```
ssm-shell [profile_name] target_spec

profile_name is the optional name of a profile configured in the local AWS configuration file.  If not set,
the AWS_PROFILE environment variable will be checked. If the environment variable is unset, credentials set
via environment variables, of the default profile credentials will be used

target_spec is a required argument of the EC2 instance ID to request a shell for.
```

If successful, a command terminal prompt will be displayed which allows you to interact with the instance.

## TODO
So far this client has only been tested on macOS and Linux systems, connecting to Linux EC2 instances.  More work
needs to be done to make sure it behaves appropriately on Windows systems (connecting to any type of EC2 instance),
and for any system trying to connect to a Windows-based EC2 instance using SSM.
