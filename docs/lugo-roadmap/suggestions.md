# Lugo — guide des fonctionnalités FiveM

## 1. Vision

Lugo doit devenir une base de connaissance sémantique d’un serveur FiveM : le même modèle doit alimenter l’éditeur, le CI et les agents MCP. Une ressource n’est pas seulement un dossier Lua ; c’est un contrat d’exécution composé d’un manifest, de scripts client/server/shared, d’assets, d’événements, d’exports, de callbacks, de permissions, de données et parfois d’une interface NUI.

## 2. Règles de conception

- **Réutiliser l’index existant** : AST, Resolver, LuaDoc, GlobalIndex, ResourceGraph et profils FiveM restent la source de vérité.
- **Confiance explicite** : toute inférence dynamique doit indiquer `certain`, `likely` ou `unknown`.
- **Pas de faux positifs coûteux** : les règles sécurité/performance sont opt-in ou informatives par défaut.
- **Scopes stricts** : client, server, shared, resource et plain Lua ne doivent jamais être mélangés.
- **Preview avant écriture** : LSP fournit des edits ; MCP les explique et les valide, mais n’écrit pas implicitement.
- **Contrats versionnés** : les changements d’événements, exports, NUI et State Bags doivent pouvoir être comparés en CI.
- **Performance préservée** : indexer les métadonnées, ne pas charger les assets inutilement, borner les réponses.

## 3. Modèle FiveM cible

Étendre le modèle pour représenter :

```text
Workspace
 ├─ ResourceGraph
 │   ├─ Resource / Manifest / dependency / provide
 │   ├─ ExecutableEntry(client|server|shared)
 │   ├─ AssetEntry(files|ui_page|data_file)
 │   ├─ EventContract
 │   ├─ ExportContract
 │   ├─ NUIContract
 │   ├─ StateBagContract
 │   └─ PermissionContract
 ├─ ModuleGraph
 ├─ SymbolIndex
 └─ DiagnosticIndex
```

Chaque contrat doit retenir son nom, ses locations, son scope, ses paramètres/valeurs connus, sa source (annotation, usage, builtin ou adapter) et sa confiance.

## 4. Fonctionnalités fonctionnelles

### 4.1 Événements typés et sécurité réseau

Indexer `AddEventHandler`, `RegisterNetEvent`, `RegisterServerEvent`, `TriggerEvent`, `TriggerServerEvent` et `TriggerClientEvent`, y compris constantes et wrappers simples. Pour chaque événement : direction, resource, profils compatibles, handler, arité, types et callers.

Diagnostics : direction invalide, registration absente, registration contradictoire, payload incompatible, événement public inutile, données client non validées et `source` lu après yield. Quick fixes : corriger la fonction, créer la registration, générer un handler ou extraire le nom en constante.

### 4.2 Manifests et filesystem

Indexer les chemins non-Lua sous forme légère. Séparer scripts et assets. Résoudre globs, `ui_page`, `files`, `data_file`, includes `@resource/path` et renommages. Ajouter links, complétion et diagnostics sur fichiers manquants, casse, traversal, globs vides, directives contradictoires, dépendances et cycles.

### 4.3 Exports et dépendances

Relier déclaration, résolution, consumer et signature. Vérifier scope client/server, dependency déclarée, alias `provide`, typo de ressource et argument contractuel. Ajouter vue de graphe et impact transitif.

### 4.4 NUI

Analyser manifest + HTML/JS/TS sans parser toute l’application web. Extraire routes `fetch`, listeners `message`, actions envoyées, callbacks Lua et focus. Fournir navigation, completion, missing route/action, réponse oubliée et payloads JSON approximatifs.

### 4.5 State Bags et OneSync

Indexer les clés et leur scope. Diagnostiquer type divergent, mutation imbriquée, réplication absente et écriture client sensible. Pour OneSync, suivre conversions handle/network ID, lifecycle, buckets et opérations après suppression avec confiance explicite.

### 4.6 Commandes, ACE, Convars et SQL

Construire un index transversal Lua + `.cfg` pour commandes, permissions et convars. Ajouter un adapter SQL optionnel pour APIs courantes, placeholders, résultats et migrations. Aucun diagnostic ne doit nécessiter une base active.

### 4.7 Frameworks et natives

Charger des packs versionnés et optionnels depuis dependencies, configuration ou détection bootstrap. Les packs ajoutent stubs, exports, callbacks, types et diagnostics sans modifier le cœur. Les natives doivent exposer scope, build, OAL, enums et dépréciations lorsque la donnée est disponible.

### 4.8 Performance et tests runtime

Fournir des hints statiques à haute confiance et importer les mesures du profiler FiveM. Séparer tests fixture et tests FXServer. Publier la version runtime et les limites de validité dans les rapports.

## 5. Fonctionnalités LSP/VS Code

- Manifests first-class et document links.
- Event/export/state/NUI completion, hover, definition, references, rename et signature help.
- Resource explorer, graphe, status bar et commandes de diagnostic.
- Quick fixes pour manifest, dépendances, direction d’event et stubs.
- Documents virtuels pour builtin events, natives, runtime et contrats.
- LuaCATS étendu et type hierarchy.
- Linked editing, CodeLens et `source.fixAll` correctement annoncés et rendus.
- Synchronisation incrémentale et diagnostics agrégés sans early return.

## 6. Fonctionnalités MCP

Le MCP doit privilégier des opérations composées plutôt que des wrappers LSP isolés :

| Outil | Rôle |
|---|---|
| `lugo_symbol_context` | Comprendre un symbole en une requête |
| `lugo_impact_analysis` | Mesurer le blast radius |
| `lugo_validate_change` | Tester un overlay sans écriture |
| `lugo_preview_rename` | Expliquer un renommage |
| `lugo_workspace_diagnostics` | Rapport borné et filtrable |
| `lugo_event_flow` | Tracer triggers et handlers |
| `lugo_export_flow` | Tracer producteurs et consommateurs |
| `lugo_manifest_explain` | Expliquer le profil d’un fichier |
| `lugo_dependency_graph` | Dépendances, cycles et impact |
| `lugo_context_bundle` | Fournir le contexte pertinent à l’agent |
| `lugo_review_change` | Revue composite d’un patch |
| `lugo_workspace_status` | Fraîcheur, génération et état de l’index |
| `lugo_capabilities` | Décrire le contrat disponible à l’agent |
| `lugo_explain_diagnostic` | Expliquer problème et corrections sûres |

Toutes les réponses doivent être structurées, paginées et bornées. Ajouter `indexGeneration`, `truncated`, `nextCursor`, `relativePath`, `sourceHash`, `confidence` et `detail`.

## 7. CI et compatibilité

Étendre `--ci` avec JSON/SARIF, baseline, allow/deny codes, seuils de sévérité, budgets de warnings et contract diff. Une modification breaking doit identifier producteurs, consommateurs, ressources et locations. Les annotations GitHub doivent être correctement échappées.

## 8. Ordre de développement recommandé

### Fondation

1. Agréger correctement tous les diagnostics et appliquer les pragmas à la fin.
2. Corriger les capacités LSP déjà implémentées mais mal annoncées/rendues.
3. Séparer scripts et assets et fiabiliser le ResourceGraph.
4. Corriger le MCP existant : prompt, range formatting, fraîcheur, allowlist et tests.

### Analyse FiveM

5. Graphe d’événements directionnel et contrats de payload.
6. Validation des manifests, assets, dépendances et cycles.
7. Exports contractuels et analyse de sécurité réseau.
8. State Bags, commandes/ACE et NUI.

### Plateforme agent

9. Structured MCP output, pagination et `workspace_status`.
10. `symbol_context`, impact, graphes et context bundles.
11. Overlay validation et workflows preview/edit.
12. Contract diff, review composite et CI SARIF.

### Écosystème

13. Packs frameworks et SQL.
14. OneSync avancé et profiler.
15. Tests runtime et contrats inter-langages.

## 9. Critères généraux d’acceptation

Chaque fonctionnalité doit fournir : modèle de données, diagnostics codés, quick fixes éventuels, documentation, fixtures positives/négatives, test plain Lua non-régressif, test client/server/shared, test d’invalidation/réindexation et sortie MCP si pertinente. Les chemins critiques doivent conserver les budgets d’allocation existants.
