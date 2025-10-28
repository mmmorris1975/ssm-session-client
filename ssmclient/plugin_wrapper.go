package ssmclient

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// Eliminated in favor of ShellSession function since AWS code appears to be poorly maintained and not working.
/* func PluginSession4(cfg aws.Config, input *ssm.StartSessionInput) error {
	out, err := ssm.NewFromConfig(cfg).StartSession(context.Background(), input)
	if err != nil {
		return err
	}

	ep, err := ssm.NewDefaultEndpointResolver().ResolveEndpoint(cfg.Region, ssm.EndpointResolverOptions{})
	if err != nil {
		return err
	}

	ssmSession := new(session.Session)
	ssmSession.Handlers.Copy() = session.Handlers{}
	ssmSession.SessionId = *out.SessionId
	ssmSession.StreamUrl = *out.StreamUrl
	ssmSession.TokenValue = *out.TokenValue
	ssmSession.Endpoint = ep.URL
	ssmSession.ClientId = uuid.NewString()
	ssmSession.TargetId = *input.Target
	ssmSession.DataChannel = &datachannel.DataChannel{}

	return ssmSession.Execute(log.Logger(false, ssmSession.ClientId))
}
*/

func PluginSession(cfg aws.Config, input *ssm.StartSessionInput) error {
	return ShellSession(cfg, *(input).Target)
}
