//go:build integration

package v1alpha1store

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/agentregistry-dev/agentregistry/pkg/api/v1alpha1"
	pkgdb "github.com/agentregistry-dev/agentregistry/pkg/registry/database"
)

func TestRemoveLocalRuntimeSeedMigrationUpgradeAndRollback(t *testing.T) {
	pool, dsn := NewTestPoolWithDSN(t, adminDSN())
	ctx := context.Background()

	migrator, err := NewOSSMigrator(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	require.NoError(t, migrator.Migrate(12))

	_, err = pool.Exec(ctx, `
		INSERT INTO runtimes (namespace, name, spec)
		VALUES
			('default', 'custom-local', '{"type":"Local"}'::jsonb),
			('default', 'custom-kubernetes', '{"type":"Kubernetes"}'::jsonb)
	`)
	require.NoError(t, err)

	require.NoError(t, migrator.Up())

	runtimes := NewMutableObjectStore(pool, TestSchema(), "runtimes")
	_, err = runtimes.GetLatest(ctx, "default", "local")
	require.ErrorIs(t, err, pkgdb.ErrNotFound)

	customLocal, err := runtimes.GetLatest(ctx, "default", "custom-local")
	require.NoError(t, err)
	var customLocalSpec v1alpha1.RuntimeSpec
	require.NoError(t, json.Unmarshal(customLocal.Spec, &customLocalSpec))
	require.Equal(t, "Local", customLocalSpec.Type)

	kubernetes, err := runtimes.GetLatest(ctx, "default", "custom-kubernetes")
	require.NoError(t, err)
	var kubernetesSpec v1alpha1.RuntimeSpec
	require.NoError(t, json.Unmarshal(kubernetes.Spec, &kubernetesSpec))
	require.Equal(t, "Kubernetes", kubernetesSpec.Type)

	require.NoError(t, migrator.Migrate(12))

	restoredLocal, err := runtimes.GetLatest(ctx, "default", "local")
	require.NoError(t, err)
	var restoredLocalSpec v1alpha1.RuntimeSpec
	require.NoError(t, json.Unmarshal(restoredLocal.Spec, &restoredLocalSpec))
	require.Equal(t, "Local", restoredLocalSpec.Type)

	_, err = runtimes.GetLatest(ctx, "default", "custom-local")
	require.NoError(t, err)
}

func TestRemoveKubernetesRuntimeSeedMigrationUpgradeAndRollback(t *testing.T) {
	pool, dsn := NewTestPoolWithDSN(t, adminDSN())
	ctx := context.Background()

	migrator, err := NewOSSMigrator(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	require.NoError(t, migrator.Migrate(13))

	runtimes := NewMutableObjectStore(pool, TestSchema(), "runtimes")
	_, err = runtimes.GetLatest(ctx, "default", "kubernetes-default")
	require.NoError(t, err)

	require.NoError(t, migrator.Up())
	_, err = runtimes.GetLatest(ctx, "default", "kubernetes-default")
	require.ErrorIs(t, err, pkgdb.ErrNotFound)

	require.NoError(t, migrator.Migrate(13))
	restored, err := runtimes.GetLatest(ctx, "default", "kubernetes-default")
	require.NoError(t, err)
	var restoredSpec v1alpha1.RuntimeSpec
	require.NoError(t, json.Unmarshal(restored.Spec, &restoredSpec))
	require.Equal(t, "Kubernetes", restoredSpec.Type)
}

func TestRemoveKubernetesRuntimeSeedMigrationPreservesModifiedOrReferencedSeed(t *testing.T) {
	t.Run("modified", func(t *testing.T) {
		pool, dsn := NewTestPoolWithDSN(t, adminDSN())
		ctx := context.Background()

		migrator, err := NewOSSMigrator(ctx, dsn)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = migrator.Close()
		})
		require.NoError(t, migrator.Migrate(13))

		_, err = pool.Exec(ctx, `
			UPDATE runtimes
			SET annotations = '{"keep":"value"}'::jsonb
			WHERE namespace = 'default' AND name = 'kubernetes-default'
		`)
		require.NoError(t, err)
		require.NoError(t, migrator.Up())

		runtimes := NewMutableObjectStore(pool, TestSchema(), "runtimes")
		modified, err := runtimes.GetLatest(ctx, "default", "kubernetes-default")
		require.NoError(t, err)
		require.Equal(t, "value", modified.Metadata.Annotations["keep"])
	})

	t.Run("referenced", func(t *testing.T) {
		pool, dsn := NewTestPoolWithDSN(t, adminDSN())
		ctx := context.Background()

		migrator, err := NewOSSMigrator(ctx, dsn)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, _ = migrator.Close()
		})
		require.NoError(t, migrator.Migrate(13))

		_, err = pool.Exec(ctx, `
			INSERT INTO deployments (namespace, name, spec)
			VALUES (
				'default',
				'uses-kubernetes-default',
				'{"targetRef":{"kind":"Agent","name":"test"},"runtimeRef":{"kind":"Runtime","name":"kubernetes-default"}}'::jsonb
			)
		`)
		require.NoError(t, err)
		require.NoError(t, migrator.Up())

		runtimes := NewMutableObjectStore(pool, TestSchema(), "runtimes")
		_, err = runtimes.GetLatest(ctx, "default", "kubernetes-default")
		require.NoError(t, err)
	})
}

func TestRemoveLocalRuntimeSeedMigrationPreservesModifiedSeed(t *testing.T) {
	pool, dsn := NewTestPoolWithDSN(t, adminDSN())
	ctx := context.Background()

	migrator, err := NewOSSMigrator(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	require.NoError(t, migrator.Migrate(12))

	_, err = pool.Exec(ctx, `
		UPDATE runtimes
		SET annotations = '{"keep":"value"}'::jsonb
		WHERE namespace = 'default' AND name = 'local'
	`)
	require.NoError(t, err)

	require.NoError(t, migrator.Up())

	runtimes := NewMutableObjectStore(pool, TestSchema(), "runtimes")
	modifiedLocal, err := runtimes.GetLatest(ctx, "default", "local")
	require.NoError(t, err)
	require.Equal(t, "value", modifiedLocal.Metadata.Annotations["keep"])

	require.NoError(t, migrator.Migrate(12))

	modifiedLocal, err = runtimes.GetLatest(ctx, "default", "local")
	require.NoError(t, err)
	require.Equal(t, "value", modifiedLocal.Metadata.Annotations["keep"])
}

func TestPluginTypeBackfillMigration(t *testing.T) {
	pool, dsn := NewTestPoolWithDSN(t, adminDSN())
	ctx := context.Background()

	migrator, err := NewOSSMigrator(ctx, dsn)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = migrator.Close()
	})
	require.NoError(t, migrator.Migrate(16))

	_, err = pool.Exec(ctx, `
		INSERT INTO plugins (namespace, name, tag, spec, content_hash, status)
		VALUES
			('default', 'legacy', 'latest',
			 '{"title":"Legacy","harnesses":["claude-code"]}'::jsonb, repeat('a', 64),
			 '{"observedGeneration":1}'::jsonb),
			('default', 'declared', 'latest',
			 '{"title":"Declared","type":"agent-plugins"}'::jsonb, repeat('b', 64),
			 '{"observedGeneration":1}'::jsonb)
	`)
	require.NoError(t, err)

	require.NoError(t, migrator.Up())

	// A generation bump would put every row ahead of its observed generation,
	// hiding it from marketplace.json and re-resolving its source on upgrade.
	readPlugin := func(name string) (v1alpha1.PluginSpec, int64, int64) {
		t.Helper()
		var raw []byte
		var generation, observed int64
		require.NoError(t, pool.QueryRow(ctx, `
			SELECT spec, generation, (status->>'observedGeneration')::bigint
			FROM plugins WHERE namespace = 'default' AND name = $1 AND tag = 'latest'
		`, name).Scan(&raw, &generation, &observed))
		var spec v1alpha1.PluginSpec
		require.NoError(t, json.Unmarshal(raw, &spec))
		return spec, generation, observed
	}

	legacySpec, legacyGeneration, legacyObserved := readPlugin("legacy")
	require.Equal(t, v1alpha1.PluginTypeClaudePlugin, legacySpec.Type, "a stored Plugin without a format gains claude-plugin")
	require.Equal(t, legacyObserved, legacyGeneration, "the backfill must not move generation")

	declaredSpec, _, _ := readPlugin("declared")
	require.Equal(t, v1alpha1.PluginTypeAgentPlugins, declaredSpec.Type, "a stored Plugin with a format keeps it")

	require.NoError(t, migrator.Migrate(16))
	rolledBack, _, _ := readPlugin("legacy")
	require.Empty(t, rolledBack.Type, "the down migration drops the format again")
}
