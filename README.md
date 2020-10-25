# SSM Session Client
A golang implementation of the protocol used with AWS SSM sessions.  The goal of this library is to provide an
easy to digest way of handling AWS SSM sessions without needing to rely on the external
[session manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html).

Simple, single stream port forwarding is available through the `ssmclient.PortForwardingSession()` function.  This
function takes an AWS SDK client.ConfigProvider type (which can be satisfied with a session.Session), and 
ssmclient.PortForwardingInput pointer (which contains the target instance and port to connect to, and the local port
to listen on).  See the [example](examples/port-forwarder) for a simple implementation.

## TODO
  * Shell sessions (linux, and possibly windows)
  * SSH over SSM sessions ()
  * Allow multiplexed connections (multiple, simultaneous streams) with port forwarding
