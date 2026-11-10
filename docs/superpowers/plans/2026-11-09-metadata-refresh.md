# Metadata Refresh Implementation Plan

> For agentic workers: use superpowers:executing-plans. Track steps here; one final independent review.

**Goal:** Reliable, richer track details and clear Worldwide source attribution.
**Architecture:** Keep /v2/track as the current contract. Parallel bounded catalog enrichment, strict Apple catalog/page matching, cached results with shorter retries for missing credits. Keep frontend session protection.
**Tech Stack:** Go, existing Spotify/MusicBrainz clients, Go HTML parser, Next.js.
**Spec:** ../specs/2026-11-09-metadata-refresh.md

## Constraints
No new member permissions, destructive migration, secrets in logs, guessed credits, or unbounded outbound requests. Existing feature branches remain isolated from main.

## Review focus
Wrong editions; same-title different artists; upstream failures and malformed pages; missing optional fields; regressions to existing cached credits. Test these at provider and handler boundaries.

## Tasks
- [x] Add automatic Apple credit client in handler/track/apple_credits.go. Tests first: exact Kygo fixture, wrong artist/album/duration/rating, ambiguous matches, mismatched page ID, malformed/oversize responses, timeout. Select one unique catalog result, parse only named credit sections, preserve source URL, merge missing roles only.
- [x] Refactor track_v2.go: retain all artists and catalog facts, optional album details, stop denied audio calls by default, run Apple and MusicBrainz within deadline, cache safely. Tests first for missing providers and retained catalog fields. Keep old JSON fields compatible.
- [x] Map metadata to frontend and list Worldwide services in pages/discover.tsx. Preserve layout; use concise disclosure and source links. Update adapter tests, run npm test and build; inspect desktop/mobile.
- [x] Investigate scheduler completion and source errors; make a bounded operational fix if supported by evidence. Document limits rather than claim false health.
- [x] Review diff independently, run Go tests/vet/build, bump release, push and deploy existing services, verify live problem tracks and social/private paths.

## Evidence and rulings
- Latest failing track: Save My Love, Kygo/Khalid/Gryffin, Spotify 5ObyGDxNWH0Uuuk3NvC5r8, ISRC SEBGA2600170, duration 210404. MusicBrainz recording exists without detailed credits.
- Apple iTunes search returns original, remix, acoustic, and unrelated same-title records. Match must not select by title alone.
- Apple public song page has structured visible credits, including songwriters, performers, guitar, producers and engineers. No lyrics or anonymous access tokens are needed.
- Daily scheduler is enabled at 05:00 UTC. Nov 9 run returned 504 after 300 seconds; Spotify new releases saved at 05:07:03 UTC.
- User already authorized implementation and production deployment; continue inline without another design approval.
- [x] Replace Reddit with Apple Music US Top 50. Test feed parsing and wrong-song rejection; add source to discovery and source disclosure. Verify Cloud Run ingestion and source contribution.


## Release evidence
- Web v1.6.1: commit 2c29982; production alias beatbrain.xyz. 54 tests and production build passed. Desktop/mobile source list and End of the World credits inspected.
- Occipital commit 33f9af4, revision occipital-00121-giv at 100%. Full Go tests, vet and build passed. Cloud check: End of the World returns seven songwriters, ten production roles and 23 performance roles; Save My Love retains writers and production. Both have Apple source links.
- Melodex commit dd558a2, revision melodex-00021-liw at 100%. Full Go tests, vet and build passed. Apple US chart ingested 39 exact Spotify matches in 16.2 seconds. The music group completed in 13.2 seconds with all five sources successful or already saved.
- Scheduler payload changed to {"target":"music"}; 05:00 UTC schedule retained, 660s attempt deadline, Cloud Run timeout 600s. Triggered actual Scheduler and confirmed HTTP 200.
- Production Discover API contains 19 Apple chart tracks among 150 ranked tracks. Frontend shows the current chart alongside other sources.
- Live guest gates, invite signup, email/password sign-in, member/music search, owner-only admin, posting, profile and sign-out passed. Temporary test records removed; three members and 32 posts remain.
- Independent review identified partial-cache retry and work-ID validation issues; regression tests reproduced and fixed both. Follow-up review caught canonical/credited artist-name compatibility; both forms now pass with one-to-one matching.
- Reddit removed at user request after API registration failed. No project change or new credentials required.
- Limits: MusicBrainz coverage can be incomplete. Apple public-page structure can change. Single-to-album fallback accepts exactly two matching album editions only when all named credits agree; ambiguous sources remain blank.
