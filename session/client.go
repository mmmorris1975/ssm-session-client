package session

import (
	"context"
	"net/http"
	"net/url"
	"os"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	awshttp "github.com/aws/aws-sdk-go-v2/aws/transport/http"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/smithy-go/logging"
	"go.uber.org/zap"
)

// BuildAWSConfig builds the AWS Config for the given service
func ProxyHttpClient() *awshttp.BuildableClient {
	if config.Flags().ProxyURL == "" {
		return awshttp.NewBuildableClient()
	}
	client := awshttp.NewBuildableClient().WithTransportOptions(func(tr *http.Transport) {
		proxyURL, err := url.Parse(config.Flags().ProxyURL)
		if err != nil {
			zap.S().Fatal(err)
		}
		tr.Proxy = http.ProxyURL(proxyURL)
	})
	return client
}
func InitializeClient() {
	if profile, ok := os.LookupEnv("AWS_PROFILE"); ok {
		config.Flags().AWSProfile = profile
	}

	if region, ok := os.LookupEnv("AWS_DEFAULT_REGION"); ok {
		config.Flags().AWSRegion = region
	}

	if region, ok := os.LookupEnv("AWS_REGION"); ok {
		config.Flags().AWSRegion = region
	}
	if config.Flags().AWSRegion == "" {
		zap.S().Fatal("AWS Region is not set")
		return
	}

	// Propagate region and profile flags to env vars so the embedded session-manager-plugin
	// (which uses the v1 AWS SDK) can pick them up alongside any other v1-based code.
	if _, ok := os.LookupEnv("AWS_DEFAULT_REGION"); !ok && config.Flags().AWSRegion != "" {
		os.Setenv("AWS_DEFAULT_REGION", config.Flags().AWSRegion)
	}
	if _, ok := os.LookupEnv("AWS_REGION"); !ok && config.Flags().AWSRegion != "" {
		os.Setenv("AWS_REGION", config.Flags().AWSRegion)
	}
	if _, ok := os.LookupEnv("AWS_PROFILE"); !ok && config.Flags().AWSProfile != "" {
		os.Setenv("AWS_PROFILE", config.Flags().AWSProfile)
	}

	if !config.IsSSMSessionManagerPluginInstalled() {
		config.Flags().UseSSMSessionPlugin = false
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_STS"); !ok && config.Flags().STSVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_STS", "https://"+config.Flags().STSVpcEndpoint)
		zap.S().Infoln("Setting STS endpoint to:", os.Getenv("AWS_ENDPOINT_URL_STS"))
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_SSM"); !ok && config.Flags().SSMVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_SSM", "https://"+config.Flags().SSMVpcEndpoint)
		zap.S().Infoln("Setting SSM endpoint to:", os.Getenv("AWS_ENDPOINT_URL_SSM"))
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_EC2"); !ok && config.Flags().EC2VpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_EC2", "https://"+config.Flags().EC2VpcEndpoint)
		zap.S().Infoln("Setting EC2 endpoint to:", os.Getenv("AWS_ENDPOINT_URL_EC2"))
	}
	if _, ok := os.LookupEnv("AWS_ENDPOINT_URL_KMS"); !ok && config.Flags().KMSVpcEndpoint != "" {
		os.Setenv("AWS_ENDPOINT_URL_KMS", "https://"+config.Flags().KMSVpcEndpoint)
		zap.S().Infoln("Setting KMS endpoint to:", os.Getenv("AWS_ENDPOINT_URL_KMS"))
	}
	if config.Flags().UseSSOLogin {
		loginInput := &SSOLoginInput{
			ProfileName: config.Flags().AWSProfile,
			Headed:      config.Flags().SSOOpenBrowser,
		}
		loginOutput, err := SSOLogin(context.Background(), loginInput)
		if err != nil {
			zap.S().Fatal("Error logging in to SSO: ", err)
		}
		if loginOutput != nil {
			zap.S().Info("SSO login successful")
		}
	}
}

func BuildAWSConfig(ctx context.Context, service string) (aws.Config, error) {

	var cfg aws.Config
	var err error

	logger := logging.LoggerFunc(func(classification logging.Classification, format string, v ...interface{}) {
		if classification == logging.Warn {
			zap.S().Warnf(format, v)
		} else {
			zap.S().Debugf(format, v)
		}
	})

	if config.Flags().AWSProfile != "" {
		cfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithSharedConfigProfile(config.Flags().AWSProfile),
			awsconfig.WithLogger(logger),
			awsconfig.WithClientLogMode((aws.LogRetries | aws.LogRequest)),
		)
	} else {
		cfg, err = awsconfig.LoadDefaultConfig(ctx,
			awsconfig.WithLogger(logger),
			awsconfig.WithClientLogMode(aws.LogRetries|aws.LogRequest),
		)
	}
	if err != nil {
		return aws.Config{}, err
	}
	if config.Flags().AWSRegion != "" {
		cfg.Region = config.Flags().AWSRegion
	}

	switch service {
	case "ssmmessages":
		if config.Flags().SSMMessagesVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "ssm":
		if config.Flags().SSMVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "ec2":
		if config.Flags().EC2VpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "kms":
		if config.Flags().KMSVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	case "sts":
		if config.Flags().STSVpcEndpoint == "" {
			cfg.HTTPClient = ProxyHttpClient()
		}
	default:
		cfg.HTTPClient = ProxyHttpClient()
	}

	return cfg, nil
}
