# Rapport agent — security and reliability opportunities

> Archive de la sortie de l’agent `security-reliability-opportunities`. Rapport read-only, aucune modification du dépôt.

## Constats vérifiés

- `lsp/diagnostics.go` contient le pipeline FiveM et un early return qui peut masquer les diagnostics Lua généraux.
- `lsp/workspace.go` indexe surtout les fichiers `.lua`, ce qui limite la validation de `files` et `ui_page`.
- Le graphe possède des champs directionnels d’événements mais la production ne les remplit pas complètement.
- `GlobalIndex` possède un tri topologique et une détection de cycles, mais leur publication dans le flux normal doit être vérifiée.
- `ci.go` échoue actuellement principalement sur les erreurs, pas sur les warnings.

## Fonctionnalités proposées

1. **Contrats d’événements directionnels** : associer `TriggerServerEvent` à des registrations serveur/shared, `TriggerClientEvent` à client/shared et conserver le local pour `TriggerEvent`. Ajouter arité, paramètres, conflits et related locations.
2. **Frontière de confiance serveur** : suivre paramètres de handlers réseau et `source` vers sinks sensibles ; reconnaître validations et permissions.
3. **`source` après yield** : suggérer une copie locale avant `Wait`/await.
4. **Séparer scripts et assets** : ne jamais donner un profil d’exécution à un Lua seulement listé dans `files`.
5. **Intégrité du graphe** : dépendances absentes, cycles, self-dependencies, aliases ambigus, globs vides, ressources dupliquées, `server_only` contradictoire et manifests concurrents.
6. **Inventaire filesystem léger** : chemins normalisés, casse, traversal, assets et globs sans conserver leur contenu.
7. **Validation des directives connues** : enum, types, doublons singleton et valeurs compatibles, tout en conservant les métadonnées custom valides.
8. **Performance prudente** : boucles sans yield, broadcasts `-1` dans des boucles, payloads volumineux et travail SQL/HTTP répété.
9. **Agrégation des diagnostics** : appliquer les pragmas après tous les passes et émettre une seule collection.
10. **CI** : `failOnSeverity`, allow/deny lists, budgets, baseline, JSON/SARIF et budgets performance.

## Priorité de fondation

Corriger d’abord l’agrégation des diagnostics et construire le graphe d’événements directionnel. Ces deux changements débloquent la sécurité, les contrats et les analyses MCP.

## Tests recommandés

Ajouter des fixtures prouvant la coexistence des diagnostics, la suppression par pragma, les directions client/serveur, les cycles, les globs vides et les ressources supprimées.
