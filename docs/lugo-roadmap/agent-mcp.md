# Rapport agent — MCP and AI-agent opportunities

> Archive de la sortie de l’agent `agent-mcp-opportunities`. Rapport read-only, aucune modification du dépôt.

## État actuel vérifié

Le serveur MCP expose les wrappers LSP, diagnostics, requête avancée, workspace, outils FiveM et reindex, ainsi que la ressource `lugo://workspace/summary`, les templates `lugo://workspace/document/{+path}` et `lugo://workspace/resource/{name}`, et le prompt `lugo_fivem_review`. Il réutilise le vrai parser, resolver, inferer, index global et graphe FiveM.

## Écarts immédiats

- Le prompt de review utilise désormais les outils enregistrés `lugo_diagnostics`, `lugo_hover`, `lugo_definition`, `lugo_references` et `lugo_workspace`; `textDocument/diagnostic` n'est pas exposé. La requête avancée est limitée à une allowlist de méthodes LSP en lecture seule.
- `lugo_range_format` ne transmet pas correctement la range.
- Les résultats sont du JSON dans `TextContent`, sans `structuredContent`, schéma de sortie, pagination ou budget.
- `lugo_lsp_request_advanced` accepte une surface trop large pour un outil supposé sûr.
- Le MCP ne vérifie pas la fraîcheur des fichiers externes et `MCPDiagnostics` modifie l’état open sans restauration apparente.
- Les éditions n’ont ni hash, ni validation de conflit, ni diff, ni delta de diagnostics.
- La release ne construit pas le binaire MCP et la documentation d’installation manque.

## Outils de haut niveau

- `lugo_symbol_context` : symbole, définition, type, LuaDoc, profil, références et callers.
- `lugo_impact_analysis` : callers, dépendants, consommateurs, ressources affectées et risque.
- `lugo_validate_workspace_edit` : hashes, stale files, overlaps, chemins, symboles et diagnostics delta.
- `lugo_preview_rename` / `lugo_preview_code_action` : diff, impact et édition bornée.
- `lugo_validate_change` : overlay virtuel avant/après, sans écriture disque.
- `lugo_workspace_diagnostics` : batch filtrable, groupé et paginé.
- `lugo_dependency_graph`, `lugo_resource_impact`, `lugo_event_flow`, `lugo_export_flow`.
- `lugo_manifest_explain`, `lugo_module_graph`, `lugo_context_bundle`, `lugo_type_explain`.
- `lugo_workspace_status`, `lugo_capabilities`, `lugo_reindex_paths`, `lugo_review_change`.

## Contrat MCP recommandé

Toutes les réponses devraient exposer `indexGeneration`, `truncated`, `nextCursor`, chemins relatifs, hashes et un niveau `detail`. Les outils d’écriture devraient rester preview-only et déléguer l’application au client agent. Utiliser des output schemas, des annotations read-only/idempotent et une allowlist de méthodes avancées.
