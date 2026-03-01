package ssmclient

import (
	"context"
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"

	"github.com/alexbacchin/ssm-session-client/config"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"go.uber.org/zap"
)

var (
	// ErrInvalidTargetFormat is the error returned if the target format doesn't match the expected format required by the resolver.
	ErrInvalidTargetFormat = errors.New("invalid target format")
	// ErrNoInstanceFound is the error returned if a resolver was unable to find an instance.
	ErrNoInstanceFound = errors.New("no instances returned from lookup")
	// ErrMultipleInstancesFound is the error returned when an alias resolves to more than one running instance.
	ErrMultipleInstancesFound = errors.New("multiple instances found for alias; target must resolve to a single instance")

	// RFC 1918 and 6598 address blocks.
	privateNets = []net.IPNet{
		{IP: net.ParseIP("10.0.0.0"), Mask: net.IPv4Mask(0xff, 0, 0, 0)},       // 10.0/8
		{IP: net.ParseIP("172.16.0.0"), Mask: net.IPv4Mask(0xff, 0xf0, 0, 0)},  // 172.16/12
		{IP: net.ParseIP("192.168.0.0"), Mask: net.IPv4Mask(0xff, 0xff, 0, 0)}, // 192.168/16
		{IP: net.ParseIP("100.64.0.0"), Mask: net.IPv4Mask(0xff, 0xc0, 0, 0)},  // 100.64/10
	}
)

// TargetResolver is the interface specification for something which knows how to resolve and EC2 instance identifier.
type TargetResolver interface {
	Resolve(string) (string, error)
}

// ResolveTarget attempts to find the instance ID of the target using a pre-defined resolution order.
// The first check will see if the target is already in the format of an EC2 instance ID.  Next, if
// the cfg parameter is not nil, checking by alias, EC2 instance tags or private IPv4 IP address is
// performed. Finally, resolving by DNS TXT record will be attempted.
func ResolveTarget(target string, cfg aws.Config) (string, error) {
	resolvers := []TargetResolver{
		NewAliasResolver(cfg, config.Flags().Aliases),
		NewTagResolver(cfg),
		NewIPResolver(cfg),
	}

	return ResolveTargetChain(strings.TrimSpace(target), append(resolvers, NewDNSResolver())...)
}

// ResolveTargetChain attempts to find the instance ID of the target using the provided list of TargetResolvers.
// The first check will always be to see if the target is already in the format of an EC2 instance ID before
// moving on to the resolution logic of the provided TargetResolvers.  If a resolver returns an error, the next
// resolver in the chain is checked, unless the error is ErrMultipleInstancesFound which stops the chain
// immediately.  If all resolvers fail to find an instance ID an error is returned.
func ResolveTargetChain(target string, resolvers ...TargetResolver) (inst string, err error) {
	var matched bool
	matched, err = regexp.MatchString(`^m?i-[[:xdigit:]]{8,}$`, target)
	if err != nil {
		return "", err
	}

	if matched {
		return target, nil
	}

	for _, res := range resolvers {
		inst, err = res.Resolve(target)
		if err == nil {
			return inst, nil
		}
		if errors.Is(err, ErrMultipleInstancesFound) {
			return "", err
		}
	}
	return "", ErrNoInstanceFound
}

// NewAliasResolver returns a TargetResolver that looks up an alias name from the provided map
// and resolves it to an EC2 instance by tag. It is strict: if more than one instance matches
// the alias tag filter, an error is returned instead of silently picking the first.
func NewAliasResolver(cfg aws.Config, aliases map[string]config.TargetAlias) *AliasResolver {
	return &AliasResolver{EC2Resolver: &EC2Resolver{cfg: cfg}, aliases: aliases}
}

// NewTagResolver is a TargetResolver which knows how to find an EC2 instance using tags.
func NewTagResolver(cfg aws.Config) *TagResolver {
	return &TagResolver{&EC2Resolver{cfg: cfg}}
}

// NewIPResolver is a TargetResolver which knows how to find an EC2 instance using the private IPv4 address.
func NewIPResolver(cfg aws.Config) *IPResolver {
	return &IPResolver{&EC2Resolver{cfg: cfg}}
}

// NewDNSResolver is a TargetResolver which knows how to find an EC2 instance using DNS TXT record lookups.
func NewDNSResolver() *DNSResolver {
	return new(DNSResolver)
}

/*
 * DNS Resolver attempts to find an instance using a DNS TXT record lookup.  The DNS record is expected
 * to resolve to the EC2 instance ID associated with the DNS name.  If the DNS record is not found, or if
 * there is nothing which looks like an EC2 instance ID in the record data, and error is returned.
 */
type DNSResolver bool

func (r *DNSResolver) Resolve(target string) (string, error) {
	rr, err := net.LookupTXT(strings.TrimSpace(target))
	if err != nil {
		return "", err
	}

	re := regexp.MustCompile(`^i-[[:xdigit:]]{8,}$`)
	for _, rec := range rr {
		if re.MatchString(rec) {
			return rec, nil
		}
	}

	return "", ErrNoInstanceFound
}

/*
 * Alias Resolver looks up the target string in the configured alias map. If a matching alias is found,
 * it queries EC2 for instances with the corresponding tag key and value. Unlike other resolvers, it
 * returns ErrMultipleInstancesFound (stopping the resolver chain) when more than one running instance
 * matches the alias filter, since an alias must refer to a unique host.
 */
type AliasResolver struct {
	*EC2Resolver
	aliases map[string]config.TargetAlias
}

func (r *AliasResolver) Resolve(target string) (string, error) {
	alias, ok := r.aliases[strings.TrimSpace(target)]
	if !ok {
		return "", ErrInvalidTargetFormat
	}

	f := types.Filter{
		Name:   aws.String(fmt.Sprintf("tag:%s", alias.TagName)),
		Values: []string{alias.TagValue},
	}
	return r.EC2Resolver.ResolveStrict(f)
}

/*
 *  Tag Resolver attempts to find an instance using instance tags.  The expected format is tag_key:tag_value
 *  (ex. hostname:web0).  If the target to resolve doesn't look like a a colon-separated tag key:value pair,
 *  or no instance is found, an error is returned.  At most, 1 instance ID is returned; if more than 1 match
 *  is found, only the 1st element of the instances list is returned.  The nature of the AWS EC2 API will not
 *  guarantee ordering of the instances list.
 */
type TagResolver struct {
	*EC2Resolver
}

func (r *TagResolver) Resolve(target string) (string, error) {
	spec := strings.SplitN(strings.TrimSpace(target), `:`, 2)
	if len(spec) < 2 {
		return "", ErrInvalidTargetFormat
	}

	f := types.Filter{
		Name:   aws.String(fmt.Sprintf(`tag:%s`, spec[0])),
		Values: []string{spec[1]},
	}
	return r.EC2Resolver.Resolve(f)
}

/*
 *  IP Resolver attempts to find an instance by its private or public IPv4 address using the EC2 API.
 *  If the target doesn't look like an IPv4 address, a DNS lookup is tried. If neither of those produce
 *  an IPv4 address, or the EC2 instance lookup fails to find an instance, an error is returned.  At most,
 *  1 instance ID is returned; if more than 1 match is found, only the 1st element of the instances list
 *  is returned.  The nature of the AWS EC2 API will not guarantee ordering of the instances list.
 */
type IPResolver struct {
	*EC2Resolver
}

func (r *IPResolver) Resolve(target string) (string, error) {
	var pubIP, privIP []string
	var targets []net.IP

	trimmed := strings.TrimSpace(target)
	ip := net.ParseIP(trimmed)
	targets = []net.IP{ip}

	if ip == nil {
		// didn't look like an IP address, attempt DNS resolution ... maybe we'll find something there
		addrs, err := net.LookupIP(trimmed)
		if err != nil {
			return "", ErrInvalidTargetFormat
		}
		targets = addrs
	}

	for _, t := range targets {
		// enforces that address is IPv4 or IPv6 address which can be represented as IPv4
		if v := t.To4(); v != nil {
			if isPrivateAddr(v) {
				privIP = append(privIP, v.String())
				continue
			}
			pubIP = append(pubIP, v.String())
		}
	}

	// must resolve at least 1 public or private IPv4 address
	if len(pubIP) < 1 && len(privIP) < 1 {
		return "", ErrInvalidTargetFormat
	}

	// prefer any public address on the instance since it's entirely possible that there may be VPCs with overlapping
	// private IP space in an account and our DescribeInstances call will match any instance with that address,
	// regardless of which VPC is resides in.  In cases where there is overlapping IP space, caller should use a more
	// specific method for finding the instance, like tags.
	f := types.Filter{
		Name:   aws.String(`private-ip-address`),
		Values: privIP,
	}
	if len(pubIP) > 0 {
		f.Name = aws.String(`ip-address`)
		f.Values = pubIP
	}

	return r.EC2Resolver.Resolve(f)
}

func isPrivateAddr(addr net.IP) bool {
	for _, n := range privateNets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}

/*
 *  EC2 Resolver calls the EC2 DescribeInstances API with a provided filter. Resolve returns at most 1
 *  instance ID; if more than 1 instance matches the filter, the 1st instance ID in the list is returned
 *  with a warning. ResolveStrict returns an error if more than 1 instance matches the filter.
 */
type EC2Resolver struct {
	cfg aws.Config
}

func (r *EC2Resolver) resolve(strict bool, filter ...types.Filter) (string, error) {
	filter = append(filter, types.Filter{Name: aws.String("instance-state-name"), Values: []string{"running"}})
	o, err := ec2.NewFromConfig(r.cfg).DescribeInstances(context.Background(), &ec2.DescribeInstancesInput{Filters: filter})
	if err != nil {
		return "", err
	}

	var firstID string
	count := 0
	for _, res := range o.Reservations {
		for _, inst := range res.Instances {
			count++
			if count == 1 {
				firstID = *inst.InstanceId
			}
		}
	}

	if count == 0 {
		return "", ErrNoInstanceFound
	}
	if strict && count > 1 {
		return "", ErrMultipleInstancesFound
	}
	if count > 1 {
		zap.S().Info("WARNING: more than 1 instance found, using 1st value")
	}
	return firstID, nil
}

func (r *EC2Resolver) Resolve(filter ...types.Filter) (string, error) {
	return r.resolve(false, filter...)
}

func (r *EC2Resolver) ResolveStrict(filter ...types.Filter) (string, error) {
	return r.resolve(true, filter...)
}
