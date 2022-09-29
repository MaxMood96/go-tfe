//go:build integration
// +build integration

package tfe

import (
	"context"
	"crypto/md5"
	"encoding/base64"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func containsStateVersion(versions []*StateVersion, item *StateVersion) bool {
	for _, sv := range versions {
		if sv.ID == item.ID {
			return true
		}
	}
	return false
}

func TestStateVersionsList(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	orgTest, orgTestCleanup := createOrganization(t, client)
	t.Cleanup(orgTestCleanup)

	wTest, wTestCleanup := createWorkspace(t, client, orgTest)
	t.Cleanup(wTestCleanup)

	svTest1, svTestCleanup1 := createStateVersion(t, client, 0, wTest)
	t.Cleanup(svTestCleanup1)
	svTest2, svTestCleanup2 := createStateVersion(t, client, 1, wTest)
	t.Cleanup(svTestCleanup2)

	t.Run("without StateVersionListOptions", func(t *testing.T) {
		svl, err := client.StateVersions.List(ctx, nil)
		assert.Nil(t, svl)
		assert.Equal(t, err, ErrRequiredStateVerListOps)
	})

	t.Run("without list options", func(t *testing.T) {
		options := &StateVersionListOptions{
			Organization: orgTest.Name,
			Workspace:    wTest.Name,
		}

		svl, err := client.StateVersions.List(ctx, options)
		require.NoError(t, err)
		require.NotEmpty(t, svl.Items)

		assert.True(t, containsStateVersion(svl.Items, svTest1), fmt.Sprintf("State Versions did not contain %s", svTest1.ID))
		assert.True(t, containsStateVersion(svl.Items, svTest2), fmt.Sprintf("State Versions did not contain %s", svTest2.ID))

		assert.Equal(t, 1, svl.CurrentPage)
		assert.Equal(t, 2, svl.TotalCount)
	})

	t.Run("with list options", func(t *testing.T) {
		// Request a page number which is out of range. The result should
		// be successful, but return no results if the paging options are
		// properly passed along.
		options := &StateVersionListOptions{
			ListOptions: ListOptions{
				PageNumber: 999,
				PageSize:   100,
			},
			Organization: orgTest.Name,
			Workspace:    wTest.Name,
		}

		svl, err := client.StateVersions.List(ctx, options)
		require.NoError(t, err)
		assert.Empty(t, svl.Items)
		assert.Equal(t, 999, svl.CurrentPage)
		assert.Equal(t, 2, svl.TotalCount)
	})

	t.Run("without an organization", func(t *testing.T) {
		options := &StateVersionListOptions{
			Workspace: wTest.Name,
		}

		svl, err := client.StateVersions.List(ctx, options)
		assert.Nil(t, svl)
		assert.Equal(t, err, ErrRequiredOrg)
	})

	t.Run("without a workspace", func(t *testing.T) {
		options := &StateVersionListOptions{
			Organization: orgTest.Name,
		}

		svl, err := client.StateVersions.List(ctx, options)
		assert.Nil(t, svl)
		assert.Equal(t, err, ErrRequiredWorkspace)
	})
}

func TestStateVersionsCreate(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	wTest, wTestCleanup := createWorkspace(t, client, nil)
	t.Cleanup(wTestCleanup)

	state, err := os.ReadFile("test-fixtures/state-version/terraform.tfstate")
	if err != nil {
		t.Fatal(err)
	}

	jsonState, err := os.ReadFile("test-fixtures/json-state/state.json")
	if err != nil {
		t.Fatal(err)
	}

	jsonStateOutputs, err := os.ReadFile("test-fixtures/json-state-outputs/everything.json")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("with valid options", func(t *testing.T) {
		ctx := context.Background()
		_, err := client.Workspaces.Lock(ctx, wTest.ID, WorkspaceLockOptions{})
		if err != nil {
			t.Fatal(err)
		}

		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			Lineage: String("741c4949-60b9-5bb1-5bf8-b14f4bb14af3"),
			MD5:     String(fmt.Sprintf("%x", md5.Sum(state))),
			Serial:  Int64(1),
			State:   String(base64.StdEncoding.EncodeToString(state)),
		})
		require.NoError(t, err)

		// Get a refreshed view of the configuration version.
		refreshed, err := client.StateVersions.Read(ctx, sv.ID)
		require.NoError(t, err)

		_, err = client.Workspaces.Unlock(ctx, wTest.ID)
		if err != nil {
			t.Fatal(err)
		}

		for _, item := range []*StateVersion{
			sv,
			refreshed,
		} {
			assert.NotEmpty(t, item.ID)
			assert.Equal(t, int64(1), item.Serial)
			assert.NotEmpty(t, item.CreatedAt)
			assert.NotEmpty(t, item.DownloadURL)
		}
	})

	t.Run("with external state representation", func(t *testing.T) {
		ctx := context.Background()
		_, err := client.Workspaces.Lock(ctx, wTest.ID, WorkspaceLockOptions{})
		if err != nil {
			t.Fatal(err)
		}

		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			Lineage:          String("741c4949-60b9-5bb1-5bf8-b14f4bb14af3"),
			MD5:              String(fmt.Sprintf("%x", md5.Sum(state))),
			Serial:           Int64(1),
			State:            String(base64.StdEncoding.EncodeToString(state)),
			JSONState:        String(base64.StdEncoding.EncodeToString(jsonState)),
			JSONStateOutputs: String(base64.StdEncoding.EncodeToString(jsonStateOutputs)),
		})
		require.NoError(t, err)

		// Get a refreshed view of the configuration version.
		refreshed, err := client.StateVersions.Read(ctx, sv.ID)
		require.NoError(t, err)

		_, err = client.Workspaces.Unlock(ctx, wTest.ID)
		if err != nil {
			t.Fatal(err)
		}

		// TODO: check state outputs for the ones we sent in JSONStateOutputs

		for _, item := range []*StateVersion{
			sv,
			refreshed,
		} {
			assert.NotEmpty(t, item.ID)
			assert.Equal(t, int64(1), item.Serial)
			assert.NotEmpty(t, item.CreatedAt)
			assert.NotEmpty(t, item.DownloadURL)
		}
	})

	t.Run("with the force flag set", func(t *testing.T) {
		ctx := context.Background()
		_, err := client.Workspaces.Lock(ctx, wTest.ID, WorkspaceLockOptions{})
		if err != nil {
			t.Fatal(err)
		}

		_, err = client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			Lineage: String("741c4949-60b9-5bb1-5bf8-b14f4bb14af3"),
			MD5:     String(fmt.Sprintf("%x", md5.Sum(state))),
			Serial:  Int64(1),
			State:   String(base64.StdEncoding.EncodeToString(state)),
		})
		require.NoError(t, err)

		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			Lineage: String("821c4747-a0b9-3bd1-8bf3-c14f4bb14be7"),
			MD5:     String(fmt.Sprintf("%x", md5.Sum(state))),
			Serial:  Int64(2),
			State:   String(base64.StdEncoding.EncodeToString(state)),
			Force:   Bool(true),
		})
		require.NoError(t, err)

		// Get a refreshed view of the configuration version.
		refreshed, err := client.StateVersions.Read(ctx, sv.ID)
		require.NoError(t, err)

		_, err = client.Workspaces.Unlock(ctx, wTest.ID)
		if err != nil {
			t.Fatal(err)
		}

		for _, item := range []*StateVersion{
			sv,
			refreshed,
		} {
			assert.NotEmpty(t, item.ID)
			assert.Equal(t, int64(2), item.Serial)
			assert.NotEmpty(t, item.CreatedAt)
			assert.NotEmpty(t, item.DownloadURL)
		}
	})

	t.Run("with a run to associate with", func(t *testing.T) {
		t.Skip("This can only be tested with the run specific token")

		rTest, rTestCleanup := createRun(t, client, wTest)
		t.Cleanup(rTestCleanup)

		ctx := context.Background()
		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			MD5:    String(fmt.Sprintf("%x", md5.Sum(state))),
			Serial: Int64(0),
			State:  String(base64.StdEncoding.EncodeToString(state)),
			Run:    rTest,
		})
		require.NoError(t, err)
		require.NotEmpty(t, sv.Run)

		// Get a refreshed view of the configuration version.
		refreshed, err := client.StateVersions.Read(ctx, sv.ID)
		require.NoError(t, err)
		require.NotEmpty(t, refreshed.Run)

		for _, item := range []*StateVersion{
			sv,
			refreshed,
		} {
			assert.NotEmpty(t, item.ID)
			assert.Equal(t, int64(0), item.Serial)
			assert.NotEmpty(t, item.CreatedAt)
			assert.NotEmpty(t, item.DownloadURL)
			assert.Equal(t, rTest.ID, item.Run.ID)
		}
	})

	t.Run("without md5 hash", func(t *testing.T) {
		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			Serial: Int64(0),
			State:  String(base64.StdEncoding.EncodeToString(state)),
		})
		assert.Nil(t, sv)
		assert.Equal(t, err, ErrRequiredM5)
	})

	t.Run("withous serial", func(t *testing.T) {
		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			MD5:   String(fmt.Sprintf("%x", md5.Sum(state))),
			State: String(base64.StdEncoding.EncodeToString(state)),
		})
		assert.Nil(t, sv)
		assert.Equal(t, err, ErrRequiredSerial)
	})

	t.Run("without state", func(t *testing.T) {
		sv, err := client.StateVersions.Create(ctx, wTest.ID, StateVersionCreateOptions{
			MD5:    String(fmt.Sprintf("%x", md5.Sum(state))),
			Serial: Int64(0),
		})
		assert.Nil(t, sv)
		assert.Equal(t, err, ErrRequiredState)
	})

	t.Run("with invalid workspace id", func(t *testing.T) {
		sv, err := client.StateVersions.Create(ctx, badIdentifier, StateVersionCreateOptions{})
		assert.Nil(t, sv)
		assert.EqualError(t, err, ErrInvalidWorkspaceID.Error())
	})
}

func TestStateVersionsRead(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	svTest, svTestCleanup := createStateVersion(t, client, 0, nil)
	t.Cleanup(svTestCleanup)

	t.Run("when the state version exists", func(t *testing.T) {
		var sv *StateVersion
		sv, err := client.StateVersions.Read(ctx, svTest.ID)
		require.NoError(t, err)

		if !sv.ResourcesProcessed {
			svRetry, err := retry(func() (interface{}, error) {
				svTest, err := client.StateVersions.Read(ctx, svTest.ID)
				require.NoError(t, err)

				if !svTest.ResourcesProcessed {
					return nil, fmt.Errorf("resources not processed %s", err)
				}

				return svTest, nil
			})

			if err != nil {
				t.Fatalf("error retrying state version read, err=%s", err)
			}

			sv = svRetry.(*StateVersion)
		}

		assert.NotEmpty(t, sv.DownloadURL)
		assert.NotEmpty(t, sv.StateVersion)
		assert.NotEmpty(t, sv.TerraformVersion)
		assert.NotEmpty(t, sv.Outputs)
	})

	t.Run("when the state version does not exist", func(t *testing.T) {
		sv, err := client.StateVersions.Read(ctx, "nonexisting")
		assert.Nil(t, sv)
		assert.Equal(t, ErrResourceNotFound, err)
	})

	t.Run("with invalid state version id", func(t *testing.T) {
		sv, err := client.StateVersions.Read(ctx, badIdentifier)
		assert.Nil(t, sv)
		assert.Equal(t, err, ErrInvalidStateVerID)
	})
}

func TestStateVersionsReadWithOptions(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	svTest, svTestCleanup := createStateVersion(t, client, 0, nil)
	t.Cleanup(svTestCleanup)

	// give TFC some time to process the statefile and extract the outputs.
	waitForSVOutputs(t, client, svTest.ID)

	t.Run("when the state version exists", func(t *testing.T) {
		curOpts := &StateVersionReadOptions{
			Include: []StateVersionIncludeOpt{SVoutputs},
		}

		sv, err := client.StateVersions.ReadWithOptions(ctx, svTest.ID, curOpts)
		require.NoError(t, err)

		assert.NotEmpty(t, sv.Outputs)
	})
}

func TestStateVersionsCurrent(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	wTest1, wTest1Cleanup := createWorkspace(t, client, nil)
	t.Cleanup(wTest1Cleanup)

	wTest2, wTest2Cleanup := createWorkspace(t, client, nil)
	t.Cleanup(wTest2Cleanup)

	svTest, svTestCleanup := createStateVersion(t, client, 0, wTest1)
	t.Cleanup(svTestCleanup)

	t.Run("when a state version exists", func(t *testing.T) {
		sv, err := client.StateVersions.ReadCurrent(ctx, wTest1.ID)
		require.NoError(t, err)

		for _, stateVersion := range []*StateVersion{svTest, sv} {
			// Don't compare the DownloadURL because it will be generated twice
			// in this test - once at creation of the configuration version, and
			// again during the GET.
			stateVersion.DownloadURL = ""

			// outputs, providers are populated only once the state has been parsed by TFC
			// which can cause the tests to fail if it doesn't happen fast enough.
			stateVersion.Outputs = nil
			stateVersion.Providers = nil
		}

		assert.Equal(t, svTest.ID, sv.ID)
	})

	t.Run("when a state version does not exist", func(t *testing.T) {
		sv, err := client.StateVersions.ReadCurrent(ctx, wTest2.ID)
		assert.Nil(t, sv)
		assert.Equal(t, ErrResourceNotFound, err)
	})

	t.Run("with invalid workspace id", func(t *testing.T) {
		sv, err := client.StateVersions.ReadCurrent(ctx, badIdentifier)
		assert.Nil(t, sv)
		assert.EqualError(t, err, ErrInvalidWorkspaceID.Error())
	})
}

func TestStateVersionsCurrentWithOptions(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	wTest1, wTest1Cleanup := createWorkspace(t, client, nil)
	t.Cleanup(wTest1Cleanup)

	svTest, svTestCleanup := createStateVersion(t, client, 0, wTest1)
	t.Cleanup(svTestCleanup)

	// give TFC some time to process the statefile and extract the outputs.
	waitForSVOutputs(t, client, svTest.ID)

	t.Run("when the state version exists", func(t *testing.T) {
		curOpts := &StateVersionCurrentOptions{
			Include: []StateVersionIncludeOpt{SVoutputs},
		}

		sv, err := client.StateVersions.ReadCurrentWithOptions(ctx, wTest1.ID, curOpts)
		require.NoError(t, err)

		assert.NotEmpty(t, sv.Outputs)
	})
}

func TestStateVersionsDownload(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	svTest, svTestCleanup := createStateVersion(t, client, 0, nil)
	t.Cleanup(svTestCleanup)

	stateTest, err := os.ReadFile("test-fixtures/state-version/terraform.tfstate")
	require.NoError(t, err)

	t.Run("when the state version exists", func(t *testing.T) {
		state, err := client.StateVersions.Download(ctx, svTest.DownloadURL)
		require.NoError(t, err)
		assert.Equal(t, stateTest, state)
	})

	t.Run("when the state version does not exist", func(t *testing.T) {
		state, err := client.StateVersions.Download(
			ctx,
			svTest.DownloadURL[:len(svTest.DownloadURL)-10]+"nonexisting",
		)
		assert.Nil(t, state)
		assert.Error(t, err)
	})

	t.Run("with an invalid url", func(t *testing.T) {
		state, err := client.StateVersions.Download(ctx, badIdentifier)
		assert.Nil(t, state)
		assert.Equal(t, ErrResourceNotFound, err)
	})
}

func TestStateVersionOutputs(t *testing.T) {
	client := testClient(t)
	ctx := context.Background()

	wTest1, wTest1Cleanup := createWorkspace(t, client, nil)
	t.Cleanup(wTest1Cleanup)

	sv, svTestCleanup := createStateVersion(t, client, 0, wTest1)
	t.Cleanup(svTestCleanup)

	// give TFC some time to process the statefile and extract the outputs.
	waitForSVOutputs(t, client, sv.ID)

	t.Run("when the state version exists", func(t *testing.T) {
		outputs, err := client.StateVersions.ListOutputs(ctx, sv.ID, nil)
		require.NoError(t, err)

		assert.NotEmpty(t, outputs.Items)

		values := map[string]interface{}{}
		for _, op := range outputs.Items {
			values[op.Name] = op.Value
		}

		// These asserts are based off of the values in
		// test-fixtures/state-version/terraform.tfstate
		assert.Equal(t, "9023256633839603543", values["test_output_string"].(string))
		assert.Equal(t, float64(5), values["test_output_number"].(float64))
		assert.Equal(t, true, values["test_output_bool"].(bool))
		assert.Equal(t, []interface{}{"us-west-1a"}, values["test_output_list_string"].([]interface{}))
		assert.Equal(t, []interface{}{float64(1), float64(2)}, values["test_output_tuple_number"].([]interface{}))
		assert.Equal(t, []interface{}{"one", "two"}, values["test_output_tuple_string"].([]interface{}))
		assert.Equal(t, map[string]interface{}{"foo": "bar"}, values["test_output_object"].(map[string]interface{}))
	})

	t.Run("with list options", func(t *testing.T) {
		options := &StateVersionOutputsListOptions{
			ListOptions: ListOptions{
				PageNumber: 999,
				PageSize:   100,
			},
		}
		outputs, err := client.StateVersions.ListOutputs(ctx, sv.ID, options)
		require.NoError(t, err)
		assert.Empty(t, outputs.Items)
		assert.Equal(t, 999, outputs.CurrentPage)

		// Based on fixture test-fixtures/state-version/terraform.tfstate
		assert.Equal(t, 7, outputs.TotalCount)
	})

	t.Run("when the state version does not exist", func(t *testing.T) {
		outputs, err := client.StateVersions.ListOutputs(ctx, "sv-999999999", nil)
		assert.Nil(t, outputs)
		assert.Error(t, err)
	})

}
