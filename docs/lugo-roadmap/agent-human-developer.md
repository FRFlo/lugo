# Rapport agent — human developer opportunities

> Archive de la sortie de l’agent `human-dev-opportunities`. Rapport read-only, aucune modification du dépôt.

## Corrections et fondations

- Annoncer linked editing et `source.fixAll` correctement.
- Restaurer les titres/commandes CodeLens.
- Empêcher les warnings FiveM de masquer les diagnostics Lua.
- Corriger le hover des événements builtin lorsque le nom est quoté.
- Ajouter `onLanguage:lua`, snippets et statut d’indexation.

## Fonctionnalités FiveM

1. Manifests first-class : complétion, filesystem links, fichiers absents, globs vides, valeurs invalides, dépendances, includes et renommages.
2. Contrats d’événements : signatures, arité, types, wrappers, constantes et renommage global.
3. Catalogue builtin généré depuis `game-events-reference.md` et documents virtuels navigables.
4. Packs optionnels ESX, QBCore, ox_lib, ox_inventory, ox_target, oxmysql et vRP.
5. Pont NUI entre `ui_page`, fichiers HTML/JS, callbacks, fetch et messages.
6. Graphe de ressources visuel et diagnostics de cycle/dépendance/provider.
7. Exports avec scope, dépendances, signatures, aliases et quick fixes.
8. Commandes, key mappings, ACE, State Bags, Convars et assistance native avancée.
9. Quick fixes : ajouter script au manifest, corriger trigger, créer event, ajouter dependency, générer export stub, migrer `__resource.lua`.

## Lua et VS Code

Étendre LuaCATS (`@enum`, `@module`, `@operator`, `@cast`, `@nodiscard`, `@async`, `@package`), ajouter déclaration/type hierarchy, liens vers manifests/includes, documents virtuels runtime et synchronisation incrémentale.

## Cohérence produit

Aligner l’identité Marketplace, le publisher, le repository, l’extension ID et la documentation d’installation.
