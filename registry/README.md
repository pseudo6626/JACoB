# JACoB Community Tabs registry

The official Community Tabs registry lives in this repository. Tab authors do not edit these files directly.

Authors build and test a normal self-contained JACoB HTML tab, then use **Tab Manager → Share**. JACoB generates a `.jacobsubmit.json` file. The author signs in to GitHub, uploads that file to the Community Tab issue form, and submits it. GitHub supplies the maintainer identity; JACoB never receives the author's GitHub credentials.

Accepted submissions are stored under:

- `registry/tabs/<package-id>.json` — current searchable metadata
- `registry/packages/<package-id>/<version>.html` — immutable HTML versions
- `registry/index.json` — generated browser index
- `registry/stats.json` — aggregate GitHub release download counts

Current categories: accessibility, bgs, combat, engineering, exploration, fleet-carrier, hud, mining, navigation, racing, trading, utility, other.

The accepted package version is mirrored to a GitHub release asset after merge. Community installs prefer that release asset so GitHub can provide an aggregate download count. JACoB falls back to the repository copy if the release mirror is temporarily unavailable. Download statistics are aggregate counts only; the registry does not receive a JACoB user ID or installation history.

Updates must be submitted from a GitHub account already listed in the package's `maintainers`. A remix receives a new package ID while retaining its upstream package/version in `derivedFrom`.

Registry PRs are validated by the permanent Community Tabs workflow. Submission HTML is treated as untrusted data and is never executed.
