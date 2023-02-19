# SSM Session Client
A golang implementation of the protocol used with AWS SSM sessions.  The goal of this library is to provide an
easy to digest way of integrating AWS SSM sessions to Go code without needing to rely on the external
[session manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html).

## Port Forwarding
Simple, single stream port forwarding is available through the `ssmclient.PortForwardingSession()` function.  This
function takes an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and a
ssmclient.PortForwardingInput pointer (which contains the target instance and port to connect to, and the local port
to listen on).  See the [example](examples/port-forwarder) for a simple implementation.

## Shell
Shell-level access to an instance can be obtained using the `ssmclient.ShellSession()` function.  This function takes
an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and a string to identify the
target to connect with.  For now, this client has only been tested on macOS and Linux, connecting to a Linux target.
See the [example](examples/ssm-shell) for a simple implementation.

Note: If you have enabled KMS encryption for Sessions, then use `ssmclient.ShellPluginSession()`.

## SSH
SSH over SSM integration can be leveraged via the `ssmclient.SshSession()` function.  Since the SSM SSH integration is
a specialized form of port forwarding, the function takes the same arguments as `ssmclient.PortForwardingSession()`.
The difference being that any LocalPort configuration is ignored, and if no RemotePort is provided, the defaule value
of 22 is used.  See the [example](examples/ssm-ssh) for a simple implementation, which can be used in the SSH
configuration to enable connecting via SSH.

This feature is meant to be used in SSH configuration files according to the
[AWS documentation](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html)
except that the ProxyCommand syntax changes to:
```
ProxyCommand sh -c "name_of_program_using_this_library profile_name %h:%p"
```
Where profile_name is the AWS configuration profile to use (you should also be able to use the AWS_PROFILE environment
variable, in which case the profile_name could be omitted), and %h:%p are standard SSH configuration substitutions for
the host and port number to connect with, and can be left as-is.


## Target Lookup Helpers
A couple of helper functions are available to assist with looking up values for EC2 instance IDs.  The
`ssmclient.ResolveTarget()` and `ssmclient.ResolveTargetChain()` functions can be used to find an instance ID
using friendlier, or better known identifying information for an instance.

The `ssmclient.ResolveTarget()` function uses a predetermined lookup order to find an instance.  If provided with a
non-nil AWS SDK client.ConfigProvider (which can be satisfied with a session.Session), instance tags, or the public
or private IPv4 address (or a DNS lookup which resolves to one of those) of the instance, can be used.  If those
avenues do not yield an instance ID, then a DNS TXT record lookup is performed.

The `ssmclient.ResolveTargetChain()` function accepts a varargs list of types implementing the TargetResolver interface
to perform the instance ID resolution.  This allows custom resolution logic to be added in case the provided mechanisms
prove insufficient.

## TODO
  * Shell sessions to Windows EC2 instances 
  * Test client code on Windows to Linux and Windows instances.
  * Allow multiplexed connections (multiple, simultaneous streams) with port forwarding
  * Robustness (retries/error recovery)

## References
The source code for the AWS SSM agent, which is a useful reference for grokking message formats, and the
expected interaction with the various services of the agent.  
https://github.com/aws/amazon-ssm-agent/tree/master/agent/session  
The `contracts` directory contains definitions of the various types used for messaging, and the `plugins`, `shell`,
and `datachannel` directories can be a useful reference for how the protocol works.

The node.js library at https://github.com/bertrandmartel/aws-ssm-session was also very instructive for seeing
an actual client-side implementation of the protocol for shell sessions.