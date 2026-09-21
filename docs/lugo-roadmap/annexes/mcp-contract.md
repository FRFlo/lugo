# Annexe — contrat MCP recommandé

## Métadonnées communes

```json
{
  "indexGeneration": 12,
  "truncated": false,
  "nextCursor": null,
  "detail": "compact",
  "sourceHash": "..."
}
```

## Sécurité

Les outils de lecture sont explicitement read-only. Les outils d’édition retournent uniquement des previews et des edits validés. L’outil avancé LSP doit être limité à une allowlist de méthodes sans mutation de workspace.

## Fraîcheur

Chaque outil indiquant des locations doit pouvoir signaler `stale`, `lastIndexedAt`, `changedFiles` et la génération de l’index. Les fichiers modifiés hors LSP doivent déclencher une réindexation ciblée ou une erreur explicite.

## Échec

Les erreurs doivent distinguer chemin absent, position invalide, document non indexé, index obsolète, analyse inconclusive et opération non supportée. Ne jamais retourner une erreur LSP opaque si un code stable peut être fourni.
