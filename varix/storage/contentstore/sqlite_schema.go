package contentstore

var sqliteInitStatements = []string{
	`CREATE TABLE IF NOT EXISTS processed (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			author TEXT NOT NULL DEFAULT '',
			processed_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS follows (
			kind TEXT NOT NULL,
			platform TEXT NOT NULL,
			platform_id TEXT NOT NULL DEFAULT '',
			locator TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			query TEXT NOT NULL DEFAULT '',
			hydration_hint TEXT NOT NULL DEFAULT '',
			author_name TEXT NOT NULL DEFAULT '',
			followed_at TEXT NOT NULL,
			last_polled_at TEXT,
			PRIMARY KEY(kind, platform, locator)
		)`,
	`CREATE TABLE IF NOT EXISTS author_subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			platform TEXT NOT NULL,
			author_name TEXT NOT NULL DEFAULT '',
			platform_id TEXT NOT NULL DEFAULT '',
			profile_url TEXT NOT NULL DEFAULT '',
			strategy TEXT NOT NULL,
			rss_url TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			last_checked_at TEXT,
			UNIQUE(platform, platform_id, profile_url, author_name)
		)`,
	`CREATE TABLE IF NOT EXISTS subscription_queries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			subscription_id INTEGER NOT NULL,
			provider TEXT NOT NULL,
			query TEXT NOT NULL,
			site_filter TEXT NOT NULL DEFAULT '',
			recency_window TEXT NOT NULL DEFAULT '',
			priority INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			UNIQUE(subscription_id, provider, query),
			FOREIGN KEY(subscription_id) REFERENCES author_subscriptions(id) ON DELETE CASCADE
		)`,
	`CREATE TABLE IF NOT EXISTS poll_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			started_at TEXT NOT NULL,
			finished_at TEXT NOT NULL,
			target_count INTEGER NOT NULL,
			discovered_count INTEGER NOT NULL,
			fetched_count INTEGER NOT NULL,
			skipped_count INTEGER NOT NULL,
			store_warning_count INTEGER NOT NULL,
			poll_warning_count INTEGER NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS poll_target_runs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			target TEXT NOT NULL,
			discovered_count INTEGER NOT NULL,
			fetched_count INTEGER NOT NULL,
			skipped_count INTEGER NOT NULL,
			warning_count INTEGER NOT NULL,
			status TEXT NOT NULL,
			error_detail TEXT NOT NULL DEFAULT '',
			FOREIGN KEY(run_id) REFERENCES poll_runs(id)
		)`,
	`CREATE TABLE IF NOT EXISTS raw_captures (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			author_name TEXT NOT NULL DEFAULT '',
			author_id TEXT NOT NULL DEFAULT '',
			posted_at TEXT,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS source_lookup_jobs (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			status TEXT NOT NULL,
			attempt_count INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id),
			FOREIGN KEY(platform, external_id) REFERENCES raw_captures(platform, external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS compile_preview_runs (
			run_id INTEGER PRIMARY KEY AUTOINCREMENT,
			pipeline TEXT NOT NULL,
			sample_scope TEXT NOT NULL,
			sample_count INTEGER NOT NULL,
			worker_count INTEGER NOT NULL,
			skip_validate INTEGER NOT NULL DEFAULT 0,
			validate_paragraph_limit INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			error_detail TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT
		)`,
	`CREATE TABLE IF NOT EXISTS compile_preview_run_items (
			item_id INTEGER PRIMARY KEY AUTOINCREMENT,
			run_id INTEGER NOT NULL,
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			url TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			error_detail TEXT NOT NULL DEFAULT '',
			extract_nodes INTEGER NOT NULL DEFAULT 0,
			relations_nodes INTEGER NOT NULL DEFAULT 0,
			relations_edges INTEGER NOT NULL DEFAULT 0,
			classify_targets INTEGER NOT NULL DEFAULT 0,
			validate_targets INTEGER NOT NULL DEFAULT 0,
			render_drivers INTEGER NOT NULL DEFAULT 0,
			render_targets INTEGER NOT NULL DEFAULT 0,
			render_paths INTEGER NOT NULL DEFAULT 0,
			payload_json TEXT NOT NULL DEFAULT '',
			mainline_markdown TEXT NOT NULL DEFAULT '',
			started_at TEXT NOT NULL,
			finished_at TEXT,
			UNIQUE(run_id, platform, external_id),
			FOREIGN KEY(run_id) REFERENCES compile_preview_runs(run_id)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_compile_preview_run_items_run
			ON compile_preview_run_items(run_id, platform, external_id)`,
	`CREATE TABLE IF NOT EXISTS compiled_outputs (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			compiled_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS llm_cache_entries (
			cache_key TEXT PRIMARY KEY,
			stage_name TEXT NOT NULL,
			prompt_hash TEXT NOT NULL,
			model TEXT NOT NULL,
			input_hash TEXT NOT NULL,
			schema_version TEXT NOT NULL DEFAULT '',
			request_json TEXT NOT NULL DEFAULT '',
			response_json TEXT NOT NULL,
			token_count INTEGER NOT NULL DEFAULT 0,
			cost_micros INTEGER NOT NULL DEFAULT 0,
			latency_ms INTEGER NOT NULL DEFAULT 0,
			hit_count INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	`CREATE INDEX IF NOT EXISTS idx_llm_cache_entries_stage_input
			ON llm_cache_entries(stage_name, model, input_hash)`,
	`CREATE TABLE IF NOT EXISTS content_subgraphs (
			subgraph_id TEXT NOT NULL,
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			compile_version TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			compiled_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS verification_results (
			platform TEXT NOT NULL,
			external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			verified_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(platform, external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS verify_queue (
			queue_id TEXT PRIMARY KEY,
			object_type TEXT NOT NULL,
			object_id TEXT NOT NULL,
			source_article_id TEXT NOT NULL,
			priority INTEGER NOT NULL DEFAULT 0,
			scheduled_at TEXT NOT NULL,
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	`CREATE INDEX IF NOT EXISTS idx_verify_queue_due
			ON verify_queue(status, scheduled_at, priority DESC)`,
	`CREATE TABLE IF NOT EXISTS verify_verdict_history (
			verdict_id INTEGER PRIMARY KEY AUTOINCREMENT,
			object_type TEXT NOT NULL,
			object_id TEXT NOT NULL,
			verdict TEXT NOT NULL,
			reason TEXT,
			evidence_refs_json TEXT NOT NULL DEFAULT '[]',
			as_of TEXT NOT NULL,
			next_verify_at TEXT,
			created_at TEXT NOT NULL
		)`,
	`CREATE INDEX IF NOT EXISTS idx_verify_verdict_history_object
			ON verify_verdict_history(object_type, object_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS user_memory_nodes (
			memory_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			node_id TEXT NOT NULL,
			node_kind TEXT NOT NULL,
			node_text TEXT NOT NULL,
			source_model TEXT NOT NULL,
			source_compiled_at TEXT NOT NULL,
			valid_from TEXT NOT NULL,
			valid_to TEXT NOT NULL,
			accepted_at TEXT NOT NULL,
			UNIQUE(user_id, source_platform, source_external_id, node_id)
		)`,
	`CREATE TABLE IF NOT EXISTS memory_posterior_states (
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			node_id TEXT NOT NULL,
			node_kind TEXT NOT NULL,
			state TEXT NOT NULL,
			diagnosis_code TEXT,
			reason TEXT,
			blocked_by_node_ids_json TEXT NOT NULL DEFAULT '[]',
			last_evaluated_at TEXT,
			last_evidence_at TEXT,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(source_platform, source_external_id, node_id)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_posterior_states_state
			ON memory_posterior_states(state)`,
	`CREATE TABLE IF NOT EXISTS memory_acceptance_events (
			event_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			trigger_type TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			source_model TEXT NOT NULL,
			source_compiled_at TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			accepted_count INTEGER NOT NULL,
			accepted_at TEXT NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS memory_organization_jobs (
			job_id INTEGER PRIMARY KEY AUTOINCREMENT,
			trigger_event_id INTEGER NOT NULL,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			started_at TEXT,
			finished_at TEXT
		)`,
	`CREATE TABLE IF NOT EXISTS memory_organization_outputs (
			output_id INTEGER PRIMARY KEY AUTOINCREMENT,
			job_id INTEGER NOT NULL UNIQUE,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS memory_content_graphs (
			memory_graph_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			root_external_id TEXT NOT NULL DEFAULT '',
			subgraph_id TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			accepted_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, source_platform, source_external_id)
		)`,
	`CREATE TABLE IF NOT EXISTS memory_content_graph_subjects (
			user_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			source_platform TEXT NOT NULL,
			source_external_id TEXT NOT NULL,
			node_count INTEGER NOT NULL DEFAULT 0,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(user_id, subject, source_platform, source_external_id),
			FOREIGN KEY(user_id, source_platform, source_external_id)
				REFERENCES memory_content_graphs(user_id, source_platform, source_external_id)
				ON DELETE CASCADE
		)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_content_graph_subjects_lookup
			ON memory_content_graph_subjects(user_id, subject, source_platform, source_external_id)`,
	`CREATE TABLE IF NOT EXISTS projection_dirty_marks (
			dirty_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL,
			layer TEXT NOT NULL,
			subject TEXT NOT NULL DEFAULT '',
			ticker TEXT NOT NULL DEFAULT '',
			horizon TEXT NOT NULL DEFAULT '',
			reason TEXT NOT NULL DEFAULT '',
			source_ref TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			dirty_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(user_id, layer, subject, ticker, horizon)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_projection_dirty_marks_pending
			ON projection_dirty_marks(status, dirty_at, user_id, layer)`,
	`CREATE INDEX IF NOT EXISTS idx_projection_dirty_marks_user_pending
			ON projection_dirty_marks(status, user_id, dirty_at, layer)`,
	`CREATE TABLE IF NOT EXISTS event_graphs (
			event_graph_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			scope TEXT NOT NULL,
			anchor_subject TEXT NOT NULL,
			time_bucket TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_graphs_user_scope_anchor_bucket
			ON event_graphs(user_id, scope, anchor_subject, time_bucket)`,
	`CREATE TABLE IF NOT EXISTS event_graph_evidence_links (
			link_id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_graph_id TEXT NOT NULL,
			subgraph_id TEXT NOT NULL,
			node_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_event_graph_evidence_unique
			ON event_graph_evidence_links(event_graph_id, subgraph_id, node_id)`,
	`CREATE TABLE IF NOT EXISTS subject_horizon_memories (
			user_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			canonical_subject TEXT NOT NULL,
			horizon TEXT NOT NULL,
			window_start TEXT NOT NULL,
			window_end TEXT NOT NULL,
			refresh_policy TEXT NOT NULL,
			next_refresh_at TEXT NOT NULL,
			input_hash TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(user_id, canonical_subject, horizon)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_subject_horizon_user_subject
			ON subject_horizon_memories(user_id, canonical_subject, horizon)`,
	`CREATE TABLE IF NOT EXISTS subject_experience_memories (
			user_id TEXT NOT NULL,
			subject TEXT NOT NULL,
			canonical_subject TEXT NOT NULL,
			horizon_set TEXT NOT NULL,
			input_hash TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			generated_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			PRIMARY KEY(user_id, canonical_subject, horizon_set)
		)`,
	`CREATE INDEX IF NOT EXISTS idx_subject_experience_user_subject
			ON subject_experience_memories(user_id, canonical_subject, horizon_set)`,
	`CREATE TABLE IF NOT EXISTS paradigms (
			paradigm_id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			driver_subject TEXT NOT NULL,
			target_subject TEXT NOT NULL,
			time_bucket TEXT NOT NULL,
			payload_json TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_paradigms_user_boundary_bucket
			ON paradigms(user_id, driver_subject, target_subject, time_bucket)`,
	`CREATE TABLE IF NOT EXISTS paradigm_evidence_links (
			link_id INTEGER PRIMARY KEY AUTOINCREMENT,
			paradigm_id TEXT NOT NULL,
			event_graph_id TEXT NOT NULL DEFAULT '',
			subgraph_id TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL
		)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_paradigm_evidence_unique
			ON paradigm_evidence_links(paradigm_id, event_graph_id, subgraph_id)`,
	`CREATE TABLE IF NOT EXISTS global_memory_organization_outputs (
			output_id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id TEXT NOT NULL UNIQUE,
			payload_json TEXT NOT NULL,
			created_at TEXT NOT NULL
		)`,
	`CREATE TABLE IF NOT EXISTS global_memory_v2_outputs (
				output_id INTEGER PRIMARY KEY AUTOINCREMENT,
				user_id TEXT NOT NULL UNIQUE,
				payload_json TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
	`CREATE TABLE IF NOT EXISTS memory_canonical_entities (
				entity_id TEXT PRIMARY KEY,
				entity_type TEXT NOT NULL,
				canonical_name TEXT NOT NULL,
				status TEXT NOT NULL,
				merge_history_json TEXT NOT NULL DEFAULT '[]',
				split_history_json TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
	`CREATE TABLE IF NOT EXISTS memory_canonical_entity_aliases (
				alias_id INTEGER PRIMARY KEY AUTOINCREMENT,
				entity_id TEXT NOT NULL,
				alias_text TEXT NOT NULL,
				created_at TEXT NOT NULL,
				FOREIGN KEY(entity_id) REFERENCES memory_canonical_entities(entity_id)
			)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_entity_alias_unique
				ON memory_canonical_entity_aliases(alias_text)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_entity_alias_entity
				ON memory_canonical_entity_aliases(entity_id)`,
	`CREATE TABLE IF NOT EXISTS memory_relations (
				relation_id TEXT PRIMARY KEY,
				driver_entity_id TEXT NOT NULL,
				target_entity_id TEXT NOT NULL,
				status TEXT NOT NULL,
				retired_at TEXT,
				superseded_by_relation_id TEXT,
				merge_history_json TEXT NOT NULL DEFAULT '[]',
				split_history_json TEXT NOT NULL DEFAULT '[]',
				lifecycle_reason TEXT,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_relations_driver_target_active
				ON memory_relations(driver_entity_id, target_entity_id)
				WHERE status IN ('active','inactive')`,
	`CREATE INDEX IF NOT EXISTS idx_memory_relations_driver
				ON memory_relations(driver_entity_id)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_relations_target
				ON memory_relations(target_entity_id)`,
	`CREATE TABLE IF NOT EXISTS memory_mechanisms (
				mechanism_id TEXT PRIMARY KEY,
				relation_id TEXT NOT NULL,
				as_of TEXT NOT NULL,
				valid_from TEXT,
				valid_to TEXT,
				confidence REAL NOT NULL,
				status TEXT NOT NULL,
				source_refs_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_mechanisms_relation_asof
				ON memory_mechanisms(relation_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS memory_mechanism_nodes (
				mechanism_node_id TEXT PRIMARY KEY,
				mechanism_id TEXT NOT NULL,
				node_type TEXT NOT NULL,
				label TEXT NOT NULL,
				backing_accepted_node_ids_json TEXT NOT NULL DEFAULT '[]',
				sort_order INTEGER,
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_mechanism_nodes_mechanism
				ON memory_mechanism_nodes(mechanism_id)`,
	`CREATE TABLE IF NOT EXISTS memory_mechanism_edges (
				mechanism_edge_id TEXT PRIMARY KEY,
				mechanism_id TEXT NOT NULL,
				from_node_id TEXT NOT NULL,
				to_node_id TEXT NOT NULL,
				edge_type TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_mechanism_edges_mechanism
				ON memory_mechanism_edges(mechanism_id)`,
	`CREATE TABLE IF NOT EXISTS memory_path_outcomes (
				path_outcome_id TEXT PRIMARY KEY,
				mechanism_id TEXT NOT NULL,
				node_path_json TEXT NOT NULL,
				outcome_polarity TEXT NOT NULL,
				outcome_label TEXT NOT NULL,
				condition_scope TEXT,
				confidence REAL NOT NULL,
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_path_outcomes_mechanism
				ON memory_path_outcomes(mechanism_id)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_path_outcomes_polarity
				ON memory_path_outcomes(outcome_polarity)`,
	`CREATE TABLE IF NOT EXISTS memory_raw_canonical_mappings (
				mapping_id INTEGER PRIMARY KEY AUTOINCREMENT,
				canonical_object_type TEXT NOT NULL,
				canonical_object_id TEXT NOT NULL,
				source_platform TEXT NOT NULL,
				source_external_id TEXT NOT NULL,
				raw_node_id TEXT NOT NULL DEFAULT '',
				raw_edge_key TEXT NOT NULL DEFAULT '',
				mapping_confidence REAL NOT NULL,
				created_at TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
	`CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_raw_canonical_mappings_unique
				ON memory_raw_canonical_mappings(canonical_object_type, canonical_object_id, source_platform, source_external_id, raw_node_id, raw_edge_key)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_raw_canonical_mappings_canonical
				ON memory_raw_canonical_mappings(canonical_object_type, canonical_object_id)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_raw_canonical_mappings_source
				ON memory_raw_canonical_mappings(source_platform, source_external_id)`,
	`CREATE TABLE IF NOT EXISTS memory_driver_aggregates (
				aggregate_id TEXT PRIMARY KEY,
				driver_entity_id TEXT NOT NULL,
				relation_ids_json TEXT NOT NULL,
				target_entity_ids_json TEXT NOT NULL,
				mechanism_labels_json TEXT NOT NULL DEFAULT '[]',
				coverage_score REAL NOT NULL,
				conflict_count INTEGER NOT NULL,
				active_conclusion_ids_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				as_of TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_driver_aggregates_driver_asof
				ON memory_driver_aggregates(driver_entity_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS memory_target_aggregates (
				aggregate_id TEXT PRIMARY KEY,
				target_entity_id TEXT NOT NULL,
				relation_ids_json TEXT NOT NULL,
				driver_entity_ids_json TEXT NOT NULL,
				mechanism_labels_json TEXT NOT NULL DEFAULT '[]',
				coverage_score REAL NOT NULL,
				conflict_count INTEGER NOT NULL,
				active_conclusion_ids_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				as_of TEXT NOT NULL,
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_target_aggregates_target_asof
				ON memory_target_aggregates(target_entity_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS memory_conflict_views (
				conflict_id TEXT PRIMARY KEY,
				scope_type TEXT NOT NULL,
				scope_id TEXT NOT NULL,
				left_path_outcome_ids_json TEXT NOT NULL,
				right_path_outcome_ids_json TEXT NOT NULL,
				conflict_reason TEXT NOT NULL,
				conflict_topic TEXT,
				status TEXT NOT NULL,
				as_of TEXT NOT NULL,
				traceability_map_json TEXT NOT NULL DEFAULT '{}',
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_conflict_views_scope_asof
				ON memory_conflict_views(scope_type, scope_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS memory_cognitive_cards (
				card_id TEXT PRIMARY KEY,
				relation_id TEXT NOT NULL,
				as_of TEXT NOT NULL,
				title TEXT NOT NULL,
				summary TEXT NOT NULL,
				mechanism_chain_json TEXT NOT NULL DEFAULT '[]',
				key_evidence_json TEXT NOT NULL DEFAULT '[]',
				conditions_json TEXT NOT NULL DEFAULT '[]',
				predictions_json TEXT NOT NULL DEFAULT '[]',
				source_refs_json TEXT NOT NULL DEFAULT '[]',
				confidence_label TEXT NOT NULL,
				trace_entry_json TEXT NOT NULL DEFAULT '[]',
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_cognitive_cards_relation_asof
				ON memory_cognitive_cards(relation_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS memory_cognitive_conclusions (
				conclusion_id TEXT PRIMARY KEY,
				source_type TEXT NOT NULL,
				source_id TEXT NOT NULL,
				headline TEXT NOT NULL,
				subheadline TEXT,
				backing_card_ids_json TEXT NOT NULL,
				core_claims_json TEXT NOT NULL DEFAULT '[]',
				traceability_status TEXT NOT NULL,
				blocked_by_conflict INTEGER NOT NULL,
				as_of TEXT NOT NULL,
				judge_model TEXT,
				judge_prompt_version TEXT,
				judge_scores_json TEXT NOT NULL DEFAULT '{}',
				judge_passed INTEGER,
				judged_at TEXT,
				created_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_cognitive_conclusions_source_asof
				ON memory_cognitive_conclusions(source_type, source_id, as_of DESC)`,
	`CREATE TABLE IF NOT EXISTS memory_top_items (
				item_id TEXT PRIMARY KEY,
				item_type TEXT NOT NULL,
				headline TEXT NOT NULL,
				subheadline TEXT,
				backing_object_id TEXT NOT NULL,
				signal_strength TEXT NOT NULL,
				as_of TEXT NOT NULL,
				updated_at TEXT NOT NULL
			)`,
	`CREATE INDEX IF NOT EXISTS idx_memory_top_items_type_asof
				ON memory_top_items(item_type, as_of DESC)`,
}

var sqliteColumnMigrations = []struct {
	table      string
	column     string
	definition string
}{
	{table: "user_memory_nodes", column: "valid_from", definition: "TEXT NOT NULL DEFAULT ''"},
	{table: "user_memory_nodes", column: "valid_to", definition: "TEXT NOT NULL DEFAULT ''"},
}
