# Audit de gestion mémoire

Date : 21 septembre 2026
Périmètre : dépôt Go complet (`ast`, `lexer`, `parser`, `semantic`, `lsp`, indexation, CI et processus longue durée).
Nature : audit suivi de corrections applicatives ciblées.

## Résumé exécutif

L'audit confirme **2 défauts critiques**, **5 élevés**, **5 moyens** et **3 faibles** liés à la mémoire ou à sa sûreté. Les risques dominants sont des entrées LSP capables d'arrêter le processus, l'absence de limite sur les sources fournies par le client et deux croissances persistantes indexées par URI.

Quatre défauts ont été reproduits par des tests temporaires, ensuite supprimés :

- panique sur dépassement signé de `Content-Length` ;
- panique sur `tabSize` négatif ;
- troncature à 65 536 éléments d'une liste AST ;
- conservation de 1 000 ressources supprimées dans l'index et son graphe.

Le race detector ne signale aucune course dans toute la suite exécutable. Le seul test exclu est bloqué par un fichier de fixture absent du dépôt. Un fuzzing du parseur a exécuté 3 777 986 cas sans panique.

## Méthode et limites

- Revue statique des cycles de vie, caches, arènes, tailles, conversions et goroutines.
- `go vet ./...` et `staticcheck ./...`.
- Tests standards et tests avec `-race` via GCC WinLibs installé localement.
- Benchmarks avec `-benchmem`, profils `alloc_space`, `alloc_objects` et `inuse_space`.
- Tests de reproduction temporaires et fuzzing borné à 60 secondes.
- Aucune ressource externe ni donnée utilisateur n'a été modifiée.

Limites :

- `TestFiveMStdlibVisibility` échoue car `lsp/stdlib/natives_client.lua` est absent. Le fichier n'a pas été généré, car cela aurait contacté un service externe et modifié le dépôt.
- Le dépôt n'avait aucune cible de fuzzing ; une cible temporaire du parseur a été utilisée puis supprimée.
- GitNexus ne connaissait pas le dépôt `lugo`. Sa reconstruction par `npx gitnexus analyze` a échoué avec `ECOMPROMISED: Lock compromised`.

## Matrice des constats

| ID | Sévérité | Confiance | Constat |
|---|---|---|---|
| M-01 | Critique | Confirmé dynamiquement | Dépassement signé de `Content-Length`, puis panique |
| M-02 | Critique | Confirmé dynamiquement | `tabSize` négatif ou immense provoque panique/OOM |
| M-03 | Élevée | Confirmé statiquement | En-tête JSON-RPC sans borne avant le saut de ligne |
| M-04 | Élevée | Confirmé statiquement | Sources LSP sans `MaxFileSize`, avec amplification mémoire |
| M-05 | Élevée | Confirmé statiquement | Imbrication de blocs sans limite de profondeur |
| M-06 | Élevée | Confirmé dynamiquement | Ressources et graphe conservés après suppression |
| M-07 | Élevée | Confirmé statiquement | Documents fermés conservés entre réindexations |
| M-08 | Moyenne | Confirmé dynamiquement | Compteurs AST `uint16` tronqués |
| M-09 | Moyenne | Confirmé statiquement | Caches URI et symlinks sans borne |
| M-10 | Moyenne | Confirmé statiquement | Capacités maximales des arènes/buffers conservées |
| M-11 | Moyenne | Confirmé statiquement | Désactivation de télémétrie empêchant sa fermeture |
| M-12 | Moyenne | Confirmé statiquement | Export debug relisant et dupliquant sans limite |
| M-13 | Faible | Confirmé statiquement | Queue de bucket `HashIndex` non effacée |
| M-14 | Faible | Confirmé statiquement | Réponse HTTP du générateur lue sans plafond |
| M-15 | Faible | Observation | Coût d'allocation élevé du démarrage à froid |

État post-correctifs : M-01 à M-15 ont reçu des protections, nettoyages ou optimisations ciblés. M-10 et M-15 sont désormais couverts par des tests de régression de capacité et d'allocation. Le bornage des lignes d'en-tête JSON-RPC a également été ajouté lors de la validation finale.

## Constats détaillés

### M-01 — Dépassement de `Content-Length` (critique)

**Preuve.** Dans `lsp/rpc.go:15-50`, la longueur est construite chiffre par chiffre dans un `int`. Un dépassement peut produire une valeur négative, contourner la limite de 100 Mio, puis atteindre `make([]byte, length)`. Le processus n'isole pas cette panique.

**Reproduction validée.** `Content-Length: 9223372036854775808` provoque bien une panique sur amd64.

**Impact.** Arrêt du serveur par un client LSP local ou une entrée redirigée malveillante.

**Recommandation.** Employer `strconv.ParseUint`, refuser signes, caractères non numériques et dépassements, puis vérifier la limite avant conversion vers `int`.

### M-02 — `tabSize` non validé (critique)

**Preuve.** `FormattingOptions.TabSize` est un `int` (`lsp/messages.go:751-754`) transmis sans validation à `NewFormatter`. `lsp/format.go:287-300` calcule ensuite `currentLineIndent * IndentSize` pour `bytes.Repeat`.

**Reproduction validée.** Une requête de formatage avec `tabSize: -1` sur un bloc indenté panique. Une valeur positive énorme peut provoquer une allocation massive ; la multiplication peut également déborder.

**Impact.** Arrêt ou épuisement mémoire à partir d'une requête LSP valide syntaxiquement.

**Recommandation.** Borner `tabSize` à une plage stricte (par exemple 1–16), contrôler la multiplication et retourner une erreur JSON-RPC.

### M-03 — Ligne d'en-tête JSON-RPC sans borne (élevée)

**Preuve.** `lsp/rpc.go:19-23` utilise `ReadBytes('\n')`. La limite du corps est évaluée uniquement après la lecture intégrale de chaque ligne.

**Impact.** Une ligne sans saut de ligne fait croître la mémoire jusqu'à épuisement. Cette croissance est transitoire, mais non bornée par l'application.

**Recommandation.** Lire les en-têtes avec une limite cumulée faible et rejeter une ligne dépassant ce plafond.

### M-04 — Sources client sans limite (élevée)

**Preuve.** `didOpen` et `didChange` passent par `lsp/workspace.go:51-101` puis `updateDocument` sans appliquer `MaxFileSize`. La limite existe pour les lectures depuis le disque. `ast.NewTree` préalloue plusieurs arènes proportionnellement à la source (`ast/ast.go:104-125`).

Pour une source proche de la limite de transport de 100 Mio, le corps JSON, la chaîne désérialisée, sa conversion en octets et les arènes coexistent. La seule arène `Nodes` peut réserver environ 280 Mio avec la taille actuelle de `Node`.

**Impact.** Amplification de plusieurs fois la taille de l'entrée, OOM possible, puis conservation des grandes capacités.

**Recommandation.** Appliquer la même limite avant toute conversion/parsing des contenus client et réduire la limite du transport indépendamment.

### M-05 — Profondeur des blocs non limitée (élevée)

**Preuve.** Les expressions sont limitées à 200 niveaux, mais la chaîne `parseBlock` → `parseStatement` → `parseDo` (et structures similaires) ne partage pas cette limite (`parser/parser.go:765-904`).

**Impact.** Une source profondément imbriquée peut faire croître la pile jusqu'à la limite fatale du runtime. M-04 permet d'envoyer cette source directement.

**Recommandation.** Généraliser un compteur de profondeur à toutes les constructions récursives et produire une erreur de parsing récupérable.

### M-06 — Ressources supprimées conservées dans l'index (élevée)

**Preuve.** `EnsureResource` crée un `ResourceScope` et deux entrées de graphe (`lsp/global_index.go:103-137`). `clearDocument` appelle seulement `EvictSource`; cette méthode libère `Source` et `AST`, mais ne retire ni `Resources[uri]`, ni les clés du graphe (`lsp/workspace.go:1322-1346`, `lsp/global_index.go:462-480`).

**Reproduction validée.** Après création puis suppression de 1 000 URI distincts, les 1 000 ressources et les 2 000 entrées de graphe restaient présentes.

**Impact.** Croissance linéaire selon le nombre cumulé d'URI, hors du budget de 256 Mio qui ne comptabilise que sources et AST.

**Recommandation.** Ajouter une suppression complète de ressource et de ses arêtes lorsque plus aucun document ou manifeste ne la référence.

### M-07 — Documents fermés conservés (élevée)

**Preuve.** `handleDidClose` retire `OpenFiles`, mais un fichier existant reste dans `Server.Documents` (`lsp/workspace.go:109-132`). `evictClosedDocumentCaches` libère quelques caches et `Tree.Source`, tout en conservant explicitement l'AST, ses arènes et le résolveur (`lsp/server.go:184-212`). La politique de `lsp/eviction.go` n'est pas intégrée au serveur de production.

**Impact.** Ouvrir puis fermer des milliers de fichiers distincts fait croître la mémoire entre deux réindexations complètes. La réindexation limite la durée de la rétention, mais n'est pas garantie.

**Recommandation.** Intégrer une LRU bornée pour les documents fermés, avec éviction complète coordonnée avec l'index global.

### M-08 — Troncature des listes AST (moyenne)

**Preuve.** `ast.Node.Count` est un `uint16`; `flushListStack` et plusieurs chemins convertissent directement une longueur (`parser/parser.go:1275-1301`). À 65 536 éléments, le compteur devient zéro.

**Reproduction validée.** Un fichier de 65 536 instructions `a = 1`, sous la limite par défaut de 4 Mio, produit un bloc racine de compteur nul alors que ses nœuds ont été alloués.

**Impact.** Corruption logique de l'AST et mémoire allouée devenue invisible aux consommateurs.

**Recommandation.** Refuser explicitement les listes trop longues ou élargir le compteur et auditer toutes les conversions similaires.

### M-09 — Caches URI et symlinks sans borne (moyenne)

`Server.uriCache` et `Server.symlinkCache` conservent chaque chemin rencontré pendant toute la vie du serveur (`lsp/workspace.go:1591-1623`). Ils ne sont ni bornés ni nettoyés lors de la suppression d'un document ou workspace. Leur croissance dépend du nombre cumulé de chemins, pas du jeu actif.

**Recommandation.** Borner ces caches ou les nettoyer avec le cycle de vie des workspaces et documents.

### M-10 — Capacités maximales conservées (moyenne)

`Tree.Reset`, `Resolver.Reset` et les buffers partagés de diagnostics/tokens utilisent `[:0]`. Ce choix est performant en régime stable, mais conserve le plus grand pic observé. Combiné à M-04 et M-07, un gros document laisse vivre de grandes arènes après réduction et fermeture.

Les buffers contenant des pointeurs, chaînes ou interfaces devraient aussi effacer leur queue avant réutilisation ou réduction.

**Correction.** `TrimOversized` libère les backing arrays AST, resolver, parser et buffers partagés au-delà de seuils bornés ; les documents fermés déclenchent aussi ce nettoyage. Les capacités normales restent réutilisables.

### M-11 — Fermeture de télémétrie conditionnée à son activation (moyenne)

`InitTelemetry` crée un `TracerProvider` avec batcher et un client PostHog. Si `SetTelemetryEnabled(false)` est appelé ensuite, `Close` retourne immédiatement et ne ferme aucune ressource (`lsp/telemetry.go:31-117`). Une erreur PostHog après création du provider laisse également ce dernier ouvert.

**Impact.** Goroutines, files et connexions internes restent actives jusqu'à l'arrêt du processus.

**Recommandation.** Séparer l'état « initialisé » de l'état « collecte activée » et toujours fermer les ressources créées.

### M-12 — Export debug non borné (moyenne)

Après éviction de `Tree.Source`, `debugExportDocumentSource` relit le fichier entier avec `os.ReadFile` (`lsp/debug_export.go:339-350`). L'export préalloue ensuite les tokens à `len(source)/4`, copie leurs textes, construit le payload JSON indenté et le convertit en chaîne.

**Impact.** Un fichier remplacé par une version très volumineuse peut provoquer plusieurs duplications et un OOM lors de l'export.

**Recommandation.** Réappliquer `MaxFileSize`, plafonner le nombre de tokens et diffuser la sortie plutôt que construire toutes les représentations simultanément.

### M-13 — Queue de bucket `HashIndex` non effacée (faible)

La suppression filtre avec `kept := entries[:0]`, mais ne met pas à `nil` la queue du tableau lorsque des entrées restent (`lsp/symbols.go:2155-2241`). Celle-ci peut conserver symboles, LuaDoc, types et chaînes jusqu'à écrasement ou réallocation.

**Recommandation.** Appeler `clear(entries[len(kept):])` avant de stocker la tranche réduite.

### M-14 — Lecture HTTP du générateur sans plafond (faible)

`cmd/rage-lua-natives/main.go:43-59` utilise `io.ReadAll` sur une réponse distante. L'impact est limité à `go generate`, mais une réponse 200 anormalement grande peut épuiser la mémoire.

**Recommandation.** Utiliser `io.LimitReader`, valider `Content-Length` et définir un timeout HTTP.

### M-15 — Allocations du démarrage à froid (faible)

Avant correction, `BenchmarkColdStart` consommait environ 11,40 Mo et 7 668–7 672 allocations/op. Après réduction des réservations AST, initialisation lazy du resolver et de `SemanticDataTable`, il mesure environ 4,58 Mo et 7 319 allocations/op. Le benchmark séparé des constructeurs mesure 1 088 octets et 16 allocations/op.

Ce résultat reste un coût d'optimisation, pas une fuite démontrée. Le budget de régression des constructeurs est fixé à 12 allocations pour le chemin `NewTree`/`NewResolver` ; le benchmark de construction complet inclut volontairement les trois constructeurs et mesure 16 allocations/op. Le profil `inuse_space` après 100 réindexations chaudes ne montre que 3,59 Mio attribués au runtime et à l'initialisation HTTP/2, sans symbole projet visible au sommet.

## Résultats dynamiques

### Tests et race detector

- `go test ./...` : tous les packages passent sauf `lsp`, bloqué par `natives_client.lua` absent.
- `CGO_ENABLED=1 go test -race ./...` : même unique échec de fixture, aucune course signalée avant l'échec.
- `CGO_ENABLED=1 go test -race ./lsp -skip '^TestFiveMStdlibVisibility$'` : succès.
- Tous les autres packages passent sous `-race`.

Conclusion : aucune course détectée dans les 115 tests exécutables. Le test exclu empêche d'affirmer que la suite suivie complète passe telle quelle.

### Benchmarks mémoire

| Benchmark | Temps observé | Octets/op | Allocations/op |
|---|---:|---:|---:|
| `NodeAt` | 52–130 ns | 0 | 0 |
| `Lexer` | 412 ns–1,16 µs | 0 | 0 |
| `Parser` | 1,24–3,57 µs | 0 | 0 |
| `Resolver` | 314–782 ns | 0 | 0 |
| `SemanticDataTable/Get` | 4,08–12,1 ns | 0 | 0 |
| `PerRequestHover` | 5,68–12,1 µs | 0 | 0 |
| `TreeDiff` | 46,6–269 µs | 30 044 | 226 |
| `ColdStart` | ~17,0 ms | 4 580 544 | 7 319 |
| `ColdStartConstructors` | ~14,7 µs | 1 088 | 16 |

Les variations de temps proviennent des exécutions parallèles par l'agent et l'auditeur principal ; les mesures d'allocation sont stables.

### Fuzzing

Une cible temporaire a alimenté le parseur avec des octets arbitraires jusqu'à 1 Mio :

- durée : 61,9 s ;
- exécutions : 3 777 986 ;
- entrées intéressantes : 491 ;
- résultat : succès, aucune panique.

Les fichiers temporaires de test et le corpus non persistant ont été retirés.

### Analyse statique automatisée

`go vet ./...` ne signale rien. `staticcheck ./...` signale des problèmes préexistants sans défaut mémoire direct : fonctions inutilisées, `break` inefficace dans `luadoc.go`, lookup de map sous-optimal dans `resolver.go` et retour conditionnel simplifiable dans `symbols.go`.

## Priorité de correction recommandée

1. Sécuriser le framing JSON-RPC et valider `tabSize` (M-01 à M-03).
2. Limiter les sources client et la profondeur du parseur (M-04, M-05).
3. Introduire une suppression complète des ressources et une LRU de documents fermés (M-06, M-07).
4. Corriger les compteurs AST et borner l'export debug (M-08, M-12).
5. Borner les caches, relâcher les capacités extrêmes et corriger le cycle de vie télémétrie (M-09 à M-11).
6. Traiter les rétentions faibles et optimiser le cold start seulement après mesure en production (M-13 à M-15).

## Faux positifs écartés

- Les workers de `warmup.go` arrêtent leur ticker et quittent sur annulation.
- Les channels de l'indexation workspace sont fermés et les workers attendus sur le chemin normal.
- `SemanticDataTable.Clear` libère map, arènes et chunks.
- `TypePool` et `PrefetchEngine` ne sont pas utilisés en production dans l'état actuel du dépôt.
- Le budget du `GlobalIndex` évince correctement les sources/AST comptabilisés ; il ne couvre simplement pas les métadonnées des constats M-06 et M-07.
- Aucun `sync.Pool` de production problématique n'a été trouvé.

## Conclusion

Les chemins cœur lexer, parser et resolver atteignent bien l'objectif zéro allocation sur leurs benchmarks usuels. Le risque mémoire principal se situe aux frontières de confiance et dans les cycles de vie longue durée du LSP. Les quatre reproductions confirment que plusieurs défauts sont exploitables avec des entrées sous contrôle du client, tandis que les profils ne montrent pas de fuite progressive dans le scénario de réindexation chaude testé.

## Correctifs appliqués

Les correctifs ont ensuite été implémentés en lots parallèles :

- parsing strict et borné de `Content-Length` ;
- validation de `tabSize` entre 1 et 16 ;
- limite `MaxFileSize` appliquée à `didOpen`, `didChange` et `updateDocument` ;
- profondeur des blocs limitée et compteurs AST élargis en `uint32` ;
- pruning des ressources globales, bornage des caches URI/symlink et nettoyage des buckets HashIndex ;
- fermeture robuste de la télémétrie ;
- limites sur l'export debug et le générateur HTTP.

Les changements incluent des tests de régression RPC, formatage, parser, index global, cycle de vie, export debug et taille maximale.

## Campagne avancée

Une seconde campagne exhaustive complète cet audit avec des harnais temporaires, isolés du code applicatif et supprimés avant la clôture. Elle ajoute : churn de 10 000 URI, rétention de capacité après un pic de 8 Mio, 5 000 documents fermés, 20 000 normalisations d'URI, export debug de 4 Mio, crashs en sous-processus, fuzzing du framing RPC, vérifications `checkptr`, compilation 386/arm64 et tests répétés du cycle de vie des workers.

Les harnais ne sont pas destinés à être conservés dans le dépôt : ils servent à obtenir des preuves reproductibles sans transformer le rapport-only en travail de produit. Ils ont tous été supprimés après les exécutions.

### Résultats de charge et de rétention

| Scénario | Résultat après GC forcé | Conclusion |
|---|---:|---|
| 10 000 URI `untitled:` créés puis supprimés | 10 000 ressources, 10 000 dépendances et 10 000 dépendants ; +5,95–5,98 Mio de tas retenu | M-06 confirmé quantitativement |
| 5 000 documents de type `file:` fermés | 5 000 documents et 5 000 ressources ; +410,8 Mio de tas retenu | M-07 est critique en capacité, pas seulement théorique |
| 20 000 URI/répertoires normalisés | deux caches à 20 000 entrées ; +5,10 Mio de tas retenu | M-09 confirmé quantitativement |
| Pic de source de 8 Mio, puis mise à jour minuscule | arènes : 839 885 nodes, 419 942 extras, 167 900 commentaires, 279 748 offsets ; tas à ~29,3 Mio jusqu'au remplacement du parser partagé | M-10 confirmé ; la rétention dépend aussi du parser partagé |
| Remplacement ultérieur du parser partagé | retour à ~0,97 Mio | les grandes arènes deviennent récupérables, mais sans seuil de purge ni garantie temporelle |
| Export debug d'une source réécrite de 4 Mio | ~83,9–84,0 Mio alloués, JSON ~4,20 Mio | M-12 confirmé : amplification ~20× en allocation cumulée |

Un profil `inuse_space` à fréquence d'échantillonnage 1, pris après les scénarios nettoyés de churn, retombe à ~80,8 Kio attribués principalement au runtime. Cette observation ne contredit pas les rétentions mesurées : chaque scénario possède son serveur local, devenu inatteignable à la fin du test. Elle confirme que les hausses sont liées au cycle de vie du serveur, non à une fuite globale du processus de test.

### Robustesse et portabilité avancées

- Trois crashs contrôlés en sous-processus sont reproductibles : dépassement `Content-Length`, `tabSize` négatif et stack overflow causé par 20 000 blocs `do/end` avec pile plafonnée à 1 Mio.
- Le fuzzing du framing RPC a trouvé indépendamment M-01 après 558 587 exécutions en 19,4 s. Son cas minimal est `Content-Length: 101700000000000000000\n\r\n`, qui atteint `makeslice: len out of range`. Le corpus temporaire a été supprimé après capture du reproducer.
- Les tests sans fixture FiveM absente passent avec `-gcflags=all=-d=checkptr=2`.
- La suite équivalente passe sous Windows 32 bits (`GOARCH=386`) ; ce résultat n'élimine pas M-01, mais vérifie les chemins ordinaires sur un `int` de 32 bits.
- `GOARCH=arm64 go build ./...` réussit.
- Le cycle warmup répété 20 fois, avec 100 démarrages annulés à chaque répétition, revient de 2 à 2 goroutines ; les tests warmup existants passent aussi 50 fois.
