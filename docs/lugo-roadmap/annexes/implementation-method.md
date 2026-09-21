# Annexe — méthode de développement

## Pour chaque feature

1. Écrire le contrat utilisateur et les limites statiques.
2. Identifier le modèle existant à réutiliser.
3. Ajouter une fixture minimale positive et négative.
4. Ajouter les tests de profils et de non-régression plain Lua.
5. Ajouter diagnostics, suppression et quick fix si nécessaire.
6. Exposer le résultat LSP puis MCP avec un schéma borné.
7. Mesurer invalidation, réindexation et allocations.
8. Documenter la confiance et les cas `unknown`.

## Vérifications obligatoires

- `gofmt` sur les fichiers Go ;
- `go test -v -race ./...` ;
- tests fixtures FiveM ciblés ;
- tests MCP via transport mémoire ;
- validation des chemins hors workspace ;
- vérification des sorties structurées et truncation ;
- revue de `git diff` et de la portée des symboles affectés.

## Règle de sécurité

Ne jamais transformer une heuristique dynamique en erreur certaine sans preuve suffisante. Préférer un hint avec confiance et explication à un diagnostic bloquant spéculatif.
