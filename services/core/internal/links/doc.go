// Package links discovers and filters the links in a fetched HTML page. It is
// shared by the crawl worker's frontier and the `links` extraction method, so
// both apply the same discovery and include/exclude rules
// (JOB_SPEC_REFERENCE.md §3.2, §6, §9).
package links
