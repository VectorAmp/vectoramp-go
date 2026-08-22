# Changelog

All notable changes to this project will be documented in this file.

This project follows semantic versioning.

## [Unreleased]

### Changed

- **Breaking:** `AskRequest.DatasetID` is replaced by `AskRequest.DatasetIDs []string`.
  `POST /intelligence/query` retired the singular `dataset_id` and answers any request carrying it
  with a 400 naming `dataset_ids` as the replacement.
- `WithDataset` now *adds* a dataset to the scope, so repeating it widens the scope. `WithDatasets`
  sets the whole scope at once.
- `WithAllDatasets` clears the scope rather than sending the retired `"all"` sentinel — an absent
  `dataset_ids` is how the API says "every dataset the caller can see".
- `Datasets.Ask` / `ds.Ask` scope to the dataset's own id via `DatasetIDs`.

### Added

- `WithDatasets(ids ...string)` for scoping one question to several datasets.

## [0.3.0] - 2026-08-20

### Added

- Add `GitHubSource` and `GitLabSource` typed source builders.
- Add `client.Sources.CreateGitHub(...)` and `client.Sources.CreateGitLab(...)` convenience methods.

## [0.2.0] - 2026-07-20

### Added

- Add typed metadata-schema fields when creating datasets.
- Add metadata-schema merge/patch and full replacement operations.
- Document create, merge, and replace schema workflows.

## [0.1.1] - 2026-07-14

### Added

- Added vector deletion helpers for dataset vectors by ID.
- Added organization OpenAI API key secret helpers and dataset creation option for OpenAI secret references.

## [0.1.0] - 2026-07-02

### Added

- Initial public-ready package baseline for VectorAmp SDK/CLI migration to GitHub.
- GitHub Actions CI workflow.
