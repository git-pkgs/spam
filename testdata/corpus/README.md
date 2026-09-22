This corpus contains 96 saved text files from 50 package releases across eight registries. The catalogues record thirteen promotional-spam labels, thirty-three legitimate controls and four excluded abuse examples. These are manually selected examples; package counts do not measure independent campaigns or registry-wide accuracy.

`seed/catalog.json` contains the initial sixteen releases. `validation/catalog.json` contains twelve releases collected later. Both sets have now been inspected during rule selection, so neither is an unseen test set. Keep related releases together using `split_group` when comparing results.

`validation/stress-catalog.json` reuses the legitimate SpamAssassin dataset package and adds its first five sorted spam-message files. It tests quoted spam inside a legitimate package and must not be counted as another package or relabelled as promotional spam.

Under `raw-text-6`, four of thirteen promotional releases across three of eight campaign groups have nonzero default scores, unchanged from `raw-text-4` and `raw-text-5`. All thirty-three legitimate releases and the quoted-spam stress view score zero. Compared with `raw-text-3`, MailKit falls from one to zero because its donation cue no longer qualifies unrelated repeated RFC links. All four previously detected promotional releases remain nonzero. This is a regression comparison on development data, not an estimate of registry-wide false-positive rates.

`fresh/catalog.json` adds four advertisements and four legitimate controls collected after the candidate rules were fixed. Labels and member selection were frozen before evaluation. Both supplement advertisements share one conservative group; essay-service links and moving-service SEO form two other observed content groups. Their operators' independence is unproven. `fresh/selection.json` records the catalogue and rule hashes at collection. This set has now been evaluated and should not be treated as unseen data for later rule changes.

`documentation/catalog.json` adds four legitimate npm guides and reference collections for comparison with the two article advertisements in `fresh`. Only `awesome-standard` contains documentation and a manifest alone; the others include configuration, code or reference data. Archive listings preserve that distinction. Labels and selected documents were fixed before evaluation, with related guides and link collections grouped conservatively. Under `raw-text-2`, both link collections scored one point for `LINK_HEAVY`, while the article advertisements scored zero. Under `raw-text-3`, link density retains its evidence at zero default weight, so all six score zero. This change also lowers the seed positive `ai-interior-design-info` from one point to zero. These examples expose a limitation of link density; the scores are not verdicts. The upstream candidates match none of these four controls in either view.

`live/catalog.json` adds ten provisional legitimate controls selected after a live release sample and follow-up inspection. It includes Helm chart manifests, separate Maven POMs, Rust and .NET dependency declarations, Python wheel metadata, and documentation with links, repeated commands and Unicode diagrams. Two AWS components share one split group, as do two fragcap crates. The CalDAV archive was observed through the Go proxy and contains Python documentation. Review was not blind to scanner output; these are previously inspected controls, not an unseen test set. Under `raw-text-3`, all ten score zero and seven retain informational signals.

Each catalogue retains package identity, collection time, source URLs, archive digest and verification status, label rationale, and selected document provenance. Document `path` values identify local `.txt` files; `sha256` and `bytes` describe their unchanged contents. Archive members retain their original paths. Separately fetched Maven POMs use `registry/pom.xml` with a document-level source URL; their hashes are observations, not verified upstream checksums. Archive filenames are provenance only. Archives and registry response files are not needed to run the corpus. Original text, including embedded notices, is preserved; package labels do not make claims about the text's license.

The fresh collection includes top-level LICENSE files as well as manifests and READMEs. RubyGems `metadata.gz` is saved as decompressed text without parsing its YAML object tags; `container` identifies files selected from the nested `data.tar.gz`. Evidence offsets refer to the saved text bytes.

The files contain untrusted package text and promotional links. Tests read them as bytes without installing packages, rendering HTML, executing code, or following links. Git attributes preserve line endings because evidence offsets and hashes refer to the original bytes.

Run the saved samples without network access after Go dependencies are available:

```sh
go run ./cmd/spam-eval -corpus testdata/corpus/seed/catalog.json
go run ./cmd/spam-eval -corpus testdata/corpus/validation/catalog.json
go run ./cmd/spam-eval -corpus testdata/corpus/validation/stress-catalog.json
go run ./cmd/spam-eval -corpus testdata/corpus/fresh/catalog.json
go run ./cmd/spam-eval -corpus testdata/corpus/documentation/catalog.json
go run ./cmd/spam-eval -corpus testdata/corpus/live/catalog.json
go test ./...
```

Labels record manual content review; a score does not determine the label. A missed positive remains positive, and a matched control remains legitimate. Add new examples with source provenance and a label rationale before evaluating rule changes against them.
