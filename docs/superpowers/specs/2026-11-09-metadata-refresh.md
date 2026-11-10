# Metadata and discovery refresh

User scope: fix repeated missing credits, use Spotify and MusicBrainz better, list Melodex services in Worldwide, and check the daily schedule. Keep the existing social site, private access, manual posting, and current data.

Implement within existing API routes. Preserve all Spotify artists and useful catalog facts. Make unavailable Spotify audio calls opt-in. Use MusicBrainz recording and linked work relationships, with bounded requests. Add automatic Apple public song-page credits when catalog identity matches title, all artists, album, duration, and content rating; reject ambiguity and changed page structure. Keep source links. Do not infer credits, scrape lyrics, use hidden tokens, or add user OAuth scopes. Preserve useful partial responses and retry source failures.

Worldwide shows the five configured music sources with links; explain Spotify and MusicBrainz metadata separately. Do not label the scheduler healthy: logs show a 5-minute timeout while writes continued to 7 minutes. Investigate this separately from catalog coverage; report source failures honestly.

## Source replacement
The user cannot register Reddit and requested another pulse source. Use Apple's public Marketing Tools US Top 50 song feed, updated by the existing daily music job. Require exact title and combined artist matches before linking Spotify records. Remove the Reddit scrape route. Keep old cached data subject to the existing seven-day cleanup. Label the US market; do not claim a global chart. This is an implementation choice under the user's standing authorization to fix and deploy.
