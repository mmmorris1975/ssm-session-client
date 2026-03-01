//go:build acceptance

package acceptance

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmtypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
)

// ssmOnlineCache records instance IDs already confirmed Online so waitForSSMReady
// returns immediately on subsequent calls.
var (
	ssmOnlineCache   = map[string]bool{}
	ssmOnlineCacheMu sync.Mutex
)

// waitForSSMReady polls DescribeInstanceInformation until the SSM agent is Online.
// Results are cached: once online, all subsequent calls return immediately.
func waitForSSMReady(t *testing.T, instanceID string) {
	t.Helper()

	ssmOnlineCacheMu.Lock()
	if ssmOnlineCache[instanceID] {
		ssmOnlineCacheMu.Unlock()
		return
	}
	ssmOnlineCacheMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Fatalf("waitForSSMReady: load AWS config: %v", err)
	}
	client := ssm.NewFromConfig(cfg)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	t.Logf("waiting for SSM agent on %s", instanceID)
	// Check once immediately before the first tick.
	if checkSSMOnline(ctx, client, instanceID) {
		markSSMOnline(instanceID)
		return
	}
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timeout waiting for SSM agent on %s", instanceID)
		case <-ticker.C:
			if checkSSMOnline(ctx, client, instanceID) {
				t.Logf("SSM agent online on %s", instanceID)
				markSSMOnline(instanceID)
				return
			}
		}
	}
}

func markSSMOnline(instanceID string) {
	ssmOnlineCacheMu.Lock()
	ssmOnlineCache[instanceID] = true
	ssmOnlineCacheMu.Unlock()
}

func checkSSMOnline(ctx context.Context, client *ssm.Client, instanceID string) bool {
	out, err := client.DescribeInstanceInformation(ctx, &ssm.DescribeInstanceInformationInput{
		Filters: []ssmtypes.InstanceInformationStringFilter{
			{Key: aws.String("InstanceIds"), Values: []string{instanceID}},
		},
	})
	if err != nil {
		return false
	}
	return len(out.InstanceInformationList) > 0 &&
		out.InstanceInformationList[0].PingStatus == ssmtypes.PingStatusOnline
}

// sessionSet is a set of SSM session IDs.
type sessionSet map[string]bool

// registerSessionLeakCheck captures a baseline of active sessions at call time
// and registers a t.Cleanup that fails the test (and terminates leaked sessions)
// if any NEW sessions are still active after the test.
func registerSessionLeakCheck(t *testing.T, instanceID string) {
	t.Helper()
	before := captureActiveSessions(t, instanceID)
	t.Cleanup(func() {
		// Allow time for sessions to terminate naturally before checking.
		// Mux sessions may need longer for the agent to clean up streams.
		time.Sleep(8 * time.Second)
		assertNoNewSessions(t, instanceID, before)
	})
}

func captureActiveSessions(t *testing.T, instanceID string) sessionSet {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Logf("WARNING: captureActiveSessions: %v", err)
		return sessionSet{}
	}
	return listActiveSessions(ctx, ssm.NewFromConfig(cfg), instanceID)
}

func listActiveSessions(ctx context.Context, client *ssm.Client, instanceID string) sessionSet {
	out, err := client.DescribeSessions(ctx, &ssm.DescribeSessionsInput{
		State: ssmtypes.SessionStateActive,
		Filters: []ssmtypes.SessionFilter{
			{Key: ssmtypes.SessionFilterKeyTargetId, Value: aws.String(instanceID)},
		},
	})
	if err != nil {
		return sessionSet{}
	}
	set := sessionSet{}
	for _, s := range out.Sessions {
		if s.SessionId != nil {
			set[*s.SessionId] = true
		}
	}
	return set
}

// terminateAllSessions terminates ALL active SSM sessions on the given instance.
// Call this at the start of tests that are sensitive to session pollution.
func terminateAllSessions(t *testing.T, instanceID string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Logf("WARNING: terminateAllSessions: %v", err)
		return
	}
	client := ssm.NewFromConfig(cfg)
	sessions := listActiveSessions(ctx, client, instanceID)
	if len(sessions) == 0 {
		return
	}
	t.Logf("terminating %d active session(s) on %s before test", len(sessions), instanceID)
	for sid := range sessions {
		id := sid
		_, _ = client.TerminateSession(ctx, &ssm.TerminateSessionInput{SessionId: &id})
	}
	// Give SSM a moment to process the terminations.
	time.Sleep(3 * time.Second)
}

func assertNoNewSessions(t *testing.T, instanceID string, before sessionSet) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(globalInfraOutputs.AWSRegion))
	if err != nil {
		t.Logf("WARNING: assertNoNewSessions: %v", err)
		return
	}
	client := ssm.NewFromConfig(cfg)

	// Retry a few times — sessions may take a moment to transition from Active
	// after the process sends TerminateSession.
	const maxRetries = 3
	var leaked []string
	for attempt := range maxRetries {
		after := listActiveSessions(ctx, client, instanceID)
		leaked = nil
		for id := range after {
			if !before[id] {
				leaked = append(leaked, id)
			}
		}
		if len(leaked) == 0 {
			return
		}
		if attempt < maxRetries-1 {
			t.Logf("session leak check: %d session(s) still active, retrying in 5s...", len(leaked))
			time.Sleep(5 * time.Second)
		}
	}

	t.Errorf("leaked %d new SSM session(s) on %s: %v", len(leaked), instanceID, leaked)
	// Terminate them to prevent pollution of subsequent tests.
	for _, sid := range leaked {
		id := sid
		_, _ = client.TerminateSession(ctx, &ssm.TerminateSessionInput{SessionId: &id})
	}
}
