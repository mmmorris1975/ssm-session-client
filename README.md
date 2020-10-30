# SSM Session Client
A golang implementation of the protocol used with AWS SSM sessions.  The goal of this library is to provide an
easy to digest way of integrating AWS SSM sessions to Go code without needing to rely on the external
[session manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html).

Simple, single stream port forwarding is available through the `ssmclient.PortForwardingSession()` function.  This
function takes an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and a
ssmclient.PortForwardingInput pointer (which contains the target instance and port to connect to, and the local port
to listen on).  See the [example](examples/port-forwarder) for a simple implementation.

Shell-level access to an instance can be obtained using the `ssmclient.ShellSession()` function.  This function takes
an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and a string to identify the
target to connect with.  For now, this client has only been tested on macOS and Linux, connecting to a Linux target.
See the [example](examples/ssm-shell) for a simple implementation.

SSH over SSM integration can be leveraged via the `ssmclient.SshSession()` function.  Since the SSM SSH integration is
a specialized form of port forwarding, the function takes the same arguments as `ssmclient.PortForwardingSession()`.
The difference being that any LocalPort configuration is ignored, and if no RemotePort is provided, the defaule value
of 22 is used.  See the [example](examples/ssm-ssh) for a simple implementation, which can be used in the SSH
configuration to enable connecting via SSH.  More information can be found in the
[AWS docs](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-getting-started-enable-ssh-connections.html)

## TODO
  * Shell sessions to Windows EC2 instances and from Windows to anywhere.
  * Allow multiplexed connections (multiple, simultaneous streams) with port forwarding
  * Robustness (retries/error recovery, out of order message handling)

## References
The source code for the AWS SSM agent, which is a useful reference for grokking message formats, and the
expected interaction with the various services of the agent.  
https://github.com/aws/amazon-ssm-agent/tree/master/agent/session  
The `contracts` directory contains definitions of the various types used for messaging, and the `plugins`, `shell`,
and `datachannel` directories can be a useful reference for how the protocol works.

The node.js library at https://github.com/bertrandmartel/aws-ssm-session was also very instructive for seeing
and actual client-side implementation of the protocol for shell sessions.