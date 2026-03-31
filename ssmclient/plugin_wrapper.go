package ssmclient

import (
	"context"
	"errors"

	"github.com/aws/session-manager-plugin/src/datachannel"
	"github.com/aws/session-manager-plugin/src/log"
	"github.com/aws/session-manager-plugin/src/sdkutil"
	"github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session"
	_ "github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/portsession"
	_ "github.com/aws/session-manager-plugin/src/sessionmanagerplugin/session/shellsession"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/google/uuid"
)

func PluginSession(cfg aws.Config, input *ssm.StartSessionInput) error {
	out, err := ssm.NewFromConfig(cfg).StartSession(context.Background(), input)
	if err != nil {
		return err
	}

	ep, err := ssm.NewDefaultEndpointResolver().ResolveEndpoint(cfg.Region, ssm.EndpointResolverOptions{})
	if err != nil {
		return err
	}

	if out.SessionId == nil || out.StreamUrl == nil || out.TokenValue == nil {
		return errors.New("StartSession response missing required fields")
	}

	if input.Target == nil {
		return errors.New("StartSession input missing Target")
	}

	sdkutil.SetRegionAndProfile(cfg.Region, "")

	ssmSession := new(session.Session)
	ssmSession.SessionId = *out.SessionId
	ssmSession.StreamUrl = *out.StreamUrl
	ssmSession.TokenValue = *out.TokenValue
	ssmSession.Endpoint = ep.URL
	ssmSession.ClientId = uuid.NewString()
	ssmSession.TargetId = *input.Target
	ssmSession.DataChannel = &datachannel.DataChannel{}

	return ssmSession.Execute(log.Logger(false, ssmSession.ClientId))
}
