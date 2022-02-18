# EC2 Instance Connect support
Start a SSH session. This program is meant to be configured as a ProxyCommand in the ssh_config file.

## Build
`go build`  
This will output a file named `ec2instance-connect` in the current directory.

## Usage
```
ec2instance-connect [profile] user@target_spec

The profile_name argument is the name of profile in the local AWS configuration to use for credentials.
If unset, it will consult the AWS_PROFILE environment variable, and if that is unset, will use credentials
set via environment variables, or from the default profile.

The user parameter should be set as the user used to connect to the remote host.  This is required by the
AWS API and the ssh command doesn't seem to pass that info to the ProxyCommand via a side channel.

The target_spec parameter is required, and is in the form of ec2_instance_id:port_number (ex: i-deadbeef:80)
```

Example ssh_config (ProxyCommand doesn't honor the %u replacement for the username):
```
Host i-*
  IdentityFile ~/.ssh/path_to_your_private_key
  ProxyCommand ec2instance-connect ec2-user@%h:%p
  User ec2-user
```