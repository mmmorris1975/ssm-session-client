# SSM SSH Shim Example
An example program which can be used when connecting to an instance using SSH via a SSM session.

## Build
`go build`  
This will output a file named `ssm-ssh` in the current directory.

## Usage
```
// ssm-ssh [profile_name] target_spec
//
// The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
// if unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
// set via environment variables, or from the default profile.
//
// The target_spec parameter is required, and is in the form of ec2_instance_id[:port_number] (ex: i-deadbeef:2222)
// The port_number argument is optional, and if not provided the default SSH port (22) is used.
```
