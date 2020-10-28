# SSM Session Client
A golang implementation of the protocol used with AWS SSM sessions.  The goal of this library is to provide an
easy to digest way of integrating AWS SSM sessions to Go code without needing to rely on the external
[session manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html).

Simple, single stream port forwarding is available through the `ssmclient.PortForwardingSession()` function.  This
function takes an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and 
ssmclient.PortForwardingInput pointer (which contains the target instance and port to connect to, and the local port
to listen on).  See the [example](examples/port-forwarder) for a simple implementation.

Shell-level access to an instance can be obtained using the `ssmclient.ShellSession()` function.  This function takes
an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and a string to identify the
target to connect with.  For now, this client has only been tested on macOS and Linux, connecting to a Linux target.
See the [example](examples/ssm-shell) for a simple implementation.

## TODO
  * Shell sessions to Windows EC2 instances and from Windows to anywhere.
  * [SSH over SSM sessions](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-sessions-start.html#sessions-start-ssh)
(AWS-StartSSHSession document)
  * Allow multiplexed connections (multiple, simultaneous streams) with port forwarding
  * Robustness (reties/error recovery, out of order message handling)

## References
The source code for the AWS SSM agent, which is a useful reference for grokking message formats, and the
expected interaction with the various services of the agent.  
https://github.com/aws/amazon-ssm-agent/tree/master/agent/session  
The `contracts` directory contains definitions of the various types used for messaging, and the `plugins`, `shell`,
and `datachannel` directories can be a useful reference for how the protocol works.

The node.js library at https://github.com/bertrandmartel/aws-ssm-session was also very instructive for seeing
and actual client-side implementation of the protocol for shell sessions.