// Package release: build-time fetch of the published demo GIF.
//
// The demo GIF is not committed to the repo — it is regenerated
// post-merge and pushed to the orphan `assets` branch (see
// .github/workflows/demo.yml). This file pulls it into the
// working tree at site-build time so the deployed site serves
// the current GIF in website/static/ as a first-party asset
// (was a runtime raw.githubusercontent <img> hotlink).
//
// The site's cross-tool benchmark numbers are deliberately NOT
// pulled here. They come from the committed in-repo snapshot
// under docs/research/benchmarks/, refreshed deliberately via
// run.sh and reviewed in a PR. The per-merge benchmark.yml run
// (which re-measures on a noisy shared runner) is a record-only
// drift signal and must not feed the published figures, or the
// site's numbers would swing run-to-run. The `bench-fragments`
// CI gate is a separate workflow that validates the committed
// snapshot against the committed JSON.
package release

import (
	"fmt"
	"path/filepath"
)

// rawAssetsBase is the raw-content root of the orphan `assets`
// branch. The branch layout is assets/<path>, so demo.gif is at
// assets/demo.gif — the same URL shape the hero <img> previously
// hotlinked at runtime.
const rawAssetsBase = "https://raw.githubusercontent.com/jeduden/mdsmith/assets/assets/"

// siteAsset is one published artifact pulled at build time. Every
// listed asset is required: a transport error or non-200 fails the
// deploy (the demo GIF is reliably published and the hero hard-depends
// on it). There is no non-required fallback branch because no asset
// needs one — add it with a test when one does (Defensive Code rule).
type siteAsset struct {
	url string
	dst string
}

// siteAssets maps each published artifact to its working-tree
// destination, resolved against the repo root.
//
// Only the demo GIF is pulled, and it is required: it is reliably
// published and the site has no committed fallback for it. The
// cross-tool benchmark numbers are intentionally not listed here
// — the site reads them from the committed in-repo snapshot under
// docs/research/benchmarks/ (refreshed via run.sh, reviewed in a
// PR), so the noisy per-merge benchmark.yml re-measurement stays a
// record-only drift signal and never moves the published figures.
func siteAssets(root string) []siteAsset {
	return []siteAsset{
		{
			url: rawAssetsBase + "demo.gif",
			dst: filepath.Join(root, "website", "static", "img", "demo.gif"),
		},
	}
}

// PullSiteAssets fetches every published artifact into the working
// tree before the Hugo build: a 200 overwrites the destination; a
// transport error or non-200 fails the deploy loudly.
func (t *Toolkit) PullSiteAssets(root string) error {
	for _, a := range siteAssets(root) {
		status, body, err := t.http.Get(a.url)
		if err != nil {
			return fmt.Errorf("fetch %s: %w", a.url, err)
		}
		if status != 200 {
			return fmt.Errorf("fetch %s: HTTP %d", a.url, status)
		}
		if err := t.fs.MkdirAll(filepath.Dir(a.dst), 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", filepath.Dir(a.dst), err)
		}
		if err := t.fs.WriteFile(a.dst, body, 0o644); err != nil {
			return fmt.Errorf("write %s: %w", a.dst, err)
		}
		fmt.Printf("pull-site-assets: pulled %s -> %s\n", a.url, a.dst)
	}
	return nil
}

// PullSiteAssets delegates to a default-OS Toolkit (see Stamp).
func PullSiteAssets(root string) error {
	return New().PullSiteAssets(root)
}
