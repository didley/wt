# Working on this repo

`main` is protected: no direct pushes, PRs required, CI must pass (test, lint, gui — ubuntu + macos), no force-pushes/deletions. Version tags (`v*`) are protected from deletion/rewrite.

When building a new feature or fix:
1. Create a new branch off `main` (don't commit directly to `main`).
2. Push it as soon as the first commit lands, and open a **draft PR** immediately — don't wait until the work is finished.
3. Mark the PR ready for review once it's complete and CI is green.
