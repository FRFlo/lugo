# Rapport agent — FiveM ecosystem opportunities

> Archive de la sortie de l’agent `fivem-ecosystem-opportunities`. Rapport read-only, aucune modification du dépôt.

## Synthèse

Le support documenté couvre déjà manifests, ressources, profils d’exécution, événements, natives et exports inter-ressources. Les ajouts les plus importants sont NUI, State Bags, commandes/ACE et OneSync, puis les adapters versionnés de frameworks/SQL, le profiling runtime et les contrats Lua–JavaScript–C#.

## Recommandations

### NUI comme contrat multi-langage

Relier `ui_page`, `files`, Lua/JS/C#, browser JavaScript, focus et callbacks. Vérifier les assets, relier `SendNUIMessage` aux listeners, relier `fetch("https://${GetParentResourceName()}/...")` à `RegisterNUICallback`, inférer les payloads et détecter les callbacks qui n’appellent pas leur réponse.

### State Bags

Indexer les clés globales, player et entity ; compléter et naviguer entre lectures/écritures/change handlers ; distinguer écritures serveur/client et réplication ; détecter mutations imbriquées non répliquées et types incompatibles.

### Commandes et ACE

Relier `RegisterCommand` aux règles `add_ace`, `remove_ace`, `add_principal` et `remove_principal` des fichiers de configuration. Diagnostiquer commandes restreintes sans règle visible, commandes sensibles publiques, doublons et héritages ambigus, avec un niveau de confiance explicite.

### OneSync

Modéliser handles et network IDs, conversions, création/suppression, ownership/migration, routing buckets et entity lockdown. Éviter les affirmations absolues sur l’ownership, qui est dynamique.

### Sécurité des événements

Détecter les handlers serveur qui font confiance aux données client : argent, inventaire, positions, identifiants ou permissions. Autoriser des annotations d’autorité et distinguer événements locaux et réseau.

### Frameworks et SQL

Proposer des packs optionnels/versionnés ESX, QBCore, ox_lib, ox_inventory, ox_target, oxmysql et vRP. Reconnaître callbacks, joueurs, jobs, items, exports et signatures SQL ; vérifier placeholders, transactions et interpolations sans nécessiter de base live.

### Runtime et tests

Compléter le profiler FiveM par mapping source, seuils CI et ingestion de métriques. Séparer fixtures statiques et tests d’intégration contre un artifact FXServer épinglé.

### Contrats inter-langages

Créer un modèle neutre pour événements, exports, NUI, callbacks et State Bags ; importer/exporter TypeScript/C# ; générer LuaCATS ; signaler les breaking changes.

## Sources de l’agent

- https://docs.fivem.net/docs/scripting-manual/nui-development/
- https://docs.fivem.net/docs/scripting-manual/nui-development/nui-callbacks/
- https://docs.fivem.net/docs/scripting-manual/networking/state-bags/
- https://docs.fivem.net/docs/scripting-manual/onesync/
- https://docs.fivem.net/docs/developers/server-security/
- https://docs.fivem.net/docs/server-manual/server-commands/
- https://docs.fivem.net/docs/scripting-manual/debugging/using-profiler/
- https://docs.esx-framework.org/ · https://docs.qbcore.org/ · https://coxdocs.dev/

## Limites

La comparaison avec Lugo était conceptuelle et non un audit symbole par symbole. Les versions de frameworks, permissions effectives, ownership OneSync, SQL et performances doivent être vérifiés avant implémentation.
