# Annexe — architecture d’implémentation cible

## Réutilisation

- `lsp/fivem.go` : manifest, profils et graphe.
- `lsp/workspace.go` : extraction AST et indexation.
- `lsp/global_index.go` : scopes et dépendances.
- `lsp/diagnostics.go` : pipeline et pragmas.
- `lsp/features.go` / `symbols.go` : hover, completion, navigation.
- `lsp/export_bridge.go` : contrats inter-ressources.
- `lsp/luadoc.go` et `infer.go` : types et signatures.
- `cmd/lugo-mcp` : adaptateur, pas moteur d’analyse.

## Évolution recommandée

Ajouter des index spécialisés attachés au `FiveMResourceGraph` plutôt que multiplier les scans workspace. Les événements, exports, NUI routes et State Bags doivent pointer vers des `ContractRef` partageant une représentation de location et de confiance.

Les assets doivent être inventoriés par chemin relatif, taille, casse normalisée et hash optionnel ; leur contenu ne doit être chargé que sur demande.

Les analyses coûteuses doivent accepter un budget : nombre de nœuds, profondeur, temps ou octets de sortie.
