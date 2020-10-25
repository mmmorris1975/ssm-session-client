# SSM Port Forwarding Example
An example program for creating an SSM Port Forwarding session.

## Build
`go build`  
This will output a file named `port-forwarder` in the current directory.

## Usage
```
port-forwarder [profile_name] target_spec

profile_name is the optional name of a profile configured in the local AWS configuration file.  If not set,
the AWS_PROFILE environment variable will be checked. If the environment variable is unset, credentials set
via environment variables, of the default profile credentials will be used

target_spec is a required argument in the form of ec2_instance_id:port_number (ex: i-deadbeef:80)
```

If session setup is successful, the following messages will be output to the terminal:
```
2020/02/02 01:23:45 listening on [::]:61905 <-- random port used for example
2020/02/02 01:23:46 Ready!
```
and the port forwarding session is available to receive data on port `61905` to send to the specified EC2 target.