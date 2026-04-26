# Project Gix - Session Memory

## Status as of 2026-03-15
- **Map Stabilization:** Fixed crashes (thread-safety), tile clipping (pixel-perfect rendering), and added Light/Dark Mode support (CartoDB Voyager/Dark Matter).
- **Global Discovery:** City search now supports fallback to OSM Nominatim (any city in the world). Results are clickable and center the map.
- **Heuristic URL Discovery:** Pasting a URL into the search bar activates a "plane" icon, which extracts name/address/GPS from the page and adds a new pin to the map.
- **UI & UX:** Rate on the chart moved higher with a background for readability. Fixed panic in vector icon drawing.
- **Security:** Updated OpenTelemetry to v1.42.0 (fix for CVE-2026-24051).

## Status as of 2026-03-21 (Bloomberg Terminal Mode & FinOps)
- **Unit Economics:** Implemented `cost-estimator` microservice and FinOps models. The system calculates scraping costs in real-time and saves to TimescaleDB, including "Heuristic Premium".
- **Event-Driven:** The application uses NATS JetStream for asynchronous transmission of `ScrapeCompletedEvent` from the server to the estimator, including stream auto-healing.
- **Cost-Limit as a Service (Guardrails):** Built `GovernanceEngine` to serve as a Circuit Breaker. Secures the budget by cutting off problematic providers based on DB queries.
- **UI Terminal:** Added "Platform FinOps Dashboard" in the Gio application. Accessible via a button in the top right or the 'F' key. Uses a native font, has an exit button with 'Esc' support, and dynamically adapts colors (Light/Dark mode). Displays micro-costs (`real_spend_24h_usd`).

## Status as of 2026-04-03 (Heuristic Discovery & UX Improvements)
- **Accurate Cantor Discovery:** Overhauled `LLMDiscoverCityCantors` to first use OSM Nominatim to find physical exchange offices (kantory) within a ~15km radius of the searched city, then uses DuckDuckGo to find specific URLs for those offices. This prevents the creation of fake cantors from generic HTML titles.
- **UI & UX:** Replaced the small loading animation over the chart with a larger, more prominent smooth trend animation with a darkened overlay to clearly indicate background search activity.
- **Cache Purging:** Updated `updateCantors` in the client to automatically remove cantors from the local state/cache if they are no longer returned by the backend, preventing "ghost cantors" from being stuck in the UI.
- **Removed Hardcoded Data:** Cleared dummy cantors from `db/seeds.sql` and the database. Removed hardcoded translations (`cantor_c1`, etc.) from all locale files so cantors now use their real discovered names.

## Next Steps
- **Priority (Continued):** Implement **Heuristic Geocoding** (extracting address from HTML -> Nominatim OSM API -> Lat/Lon on the map). This will generate real, calculable costs linked to the new FinOps system.
- Deploy and test the entire updated stack on the Digital Ocean cluster (pushing new images to the Hub).
- Expand Heuristic Scraper to pull exchange rates directly from new discoveries (URL -> Rate).
