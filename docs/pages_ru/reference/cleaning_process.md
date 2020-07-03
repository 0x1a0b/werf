---
title: Очистка
sidebar: documentation
permalink: documentation/reference/cleaning_process.html
author: Artem Kladov <artem.kladov@flant.com>, Timofey Kirillov <timofey.kirillov@flant.com>
---

В процессе сборки и публикации образов приложений Docker-слои создаются, но никогда не удаляются. 
Как следствие, [хранилище стадий]({{ site.baseurl }}/documentation/reference/stages_and_images.html#хранилище-стадий) и репозиторий образов в Docker registry постоянно увеличивается в размерах, требуя все больше и больше ресурсов. 
Прерванная сборка образа также оставляет после себя образы, которые уже никогда не будут использоваться. 

Таким образом, необходимо периодически производить очистку от неиспользуемых образов как _хранилища стадий_, так и Docker registry.

В werf реализован эффективный многоуровневый алгоритм очистки образов, использующий следующие подходы:

1. [**Очистка по политикам**](#очистка-по-политикам)
1. [**Ручная очистка**](#ручная-очистка)
1. [**Очистка хоста**](#очистка-хоста)

## Очистка по политикам

Этот метод очистки рассчитан на периодический запуск по расписанию. 
Удаление производится в соответствии с принятыми политиками очистки и является безопасной процедурой, т.к. при его запуске используются блокировки, а также игнорируются используемые в кластере образы.

Очистка по политикам состоит из двух шагов и выполняется в следующем порядке:
1. [**Очистка образов**](#очистка-образов) очищает Docker registry от неактуальных образов в соответствии с политиками очистки.
2. [**Очистка хранилища стадий**](#очистка-хранилища-стадий) синхронизирует состояние _хранилища стадий_ с существующими образами в Docker registry.

Оба указанных способа очистки объединены в команде [werf cleanup]({{ site.baseurl }}/documentation/cli/main/cleanup.html), при которой сначала выполняется удаление неактуальных образов проекта, а затем синхронизация хранилища стадий.

Docker registry является главным источником информации об образах, поэтому в первую очередь выполняется очистка образов, а уже потом _хранилища стадий_.

### Очистка образов 

> В версиях `<v1.2` по умолчанию используется старый алгоритм очистки. Алгоритм, описанный далее, будет единственным, начиная с версии `v1.2`. Для принудительного включения алгоритма необходимо использовать опцию `--git-history-based-cleanup`

#### Алгоритм работы

Логика базируется на том, что конечный образ связан с определённой [сигнатурой стадий образа]({{ site.baseurl }}/documentation/reference/stages_and_images.html#сигнатура-стадий-образа) и в [хранилище стадий]({{ site.baseurl }}/documentation/reference/stages_and_images.html#хранилище-стадий) сохраняется информация о коммитах, на которых выполнялась публикация образов с этой сигнатурой. Таким образом, обеспечивается связь тегов с историей git и появляется возможность организации эффективной очистки неактуальных образов на основе состояния git и выбранных политик.

Рассмотрим основные шаги алгоритма очистки:

- [Получение необходимых для очистки данных из хранилища стадий](#используемые-при-очистке-данные-хранилища-стадий).
    - все когда-либо собираемые [имена образов]({{ site.baseurl }}/documentation/configuration/stapel_image/naming.html);
    - набор связок [сигнатура стадий образа]({{ site.baseurl }}/documentation/reference/stages_and_images.html#сигнатура-стадий-образа) и коммит.
- Получение манифестов для всех тегов.
- Подготовка набора для очистки:
    - [теги используемые в Kubernetes](#игнорирование-используемых-в-кластере-kubernetes-образов) игнорируются;
    - теги, опубликованные версией `<v1.1.20`, игнорируются (с версии `v1.2` удаляются, можно форсировать поведение, используя опцию `--git-history-based-cleanup-v1.2`).
- Подготовка данных для сканирования:
    - сгруппированные теги по сигнатуре стадий образа __(1)__;
    - cгруппированные коммиты по сигнатуре стадий образа __(2)__;
    - [ветки и теги на основе пользовательских политик](#пользовательские-политики) __(3)__. 
- Поиск коммитов __(2)__ по истории git __(3)__. 
- Удаление тегов __(1)__ для [сигнатур стадий образа]({{ site.baseurl }}/documentation/reference/stages_and_images.html#сигнатура-стадий-образа), которые не были найдены при сканировании.

#### Пользовательские политики

Пользователь может регулировать диапазон сканирования истории git, используя [секцию cleanup в meta секции werf.yaml]({{ site.baseurl }}/documentation/configuration/cleanup.html). При отсутствии политик очистки будeт использован [набор по умолчанию]({{ site.baseurl }}/documentation/configuration/cleanup.html#политики-по-умолчанию).

Стоит отметить, что алгоритм сканирует локальное состояние git репозитория и актуальность git-веток и git-тегов крайне важна. Для синхронизации состояния git можно воспользоваться опцией `--git-history-synchronization`, которая по умолчанию включена при запуске в CI системах.

#### Используемые при очистке данные хранилища стадий   

Для оптимизации и решения специфичных кейсов, werf при работе сохраняет дополнительные данные в [хранилище стадий]({{ site.baseurl }}/documentation/reference/stages_and_images.html#хранилище-стадий). Среди таких данных мета-образы, хранящие связку [сигнатуры стадий образа]({{ site.baseurl }}/documentation/reference/stages_and_images.html#сигнатура-стадий-образа) и коммита, на котором выполнялась публикация, а также [имена образов]({{ site.baseurl }}/documentation/configuration/stapel_image/naming.html), которые когда-либо собирались.

Информация о коммитах является единственным источником правды при работе алгоритма, поэтому теги без подобной информации обрабатываются отдельно. Теги, опубликованные версией `<v1.1.20`, игнорируются (с версии `v1.2` удаляются, можно форсировать поведение, используя опцию `--git-history-based-cleanup-v1.2`).

При организации автоматической очистки команда `werf cleanup` выполняется либо по расписанию, либо вручную по случаю. Чтобы избежать удаления рабочего кеша при добавлении/удалении образов в `werf.yaml` в соседних git-ветках, при сборке в [хранилище стадий]({{ site.baseurl }}/documentation/reference/stages_and_images.html#хранилище-стадий) добавляется имя собираемого образа. Используя набор команд `werf managed-images ls|add|rm`, пользователь может редактировать, так называемый набор _managed images_.

#### Игнорирование используемых в кластере Kubernetes образов

Пока в кластере Kubernetes существует объект использующий образ, он никогда не удалится из Docker registry. Другими словами, если что-то было запущено в вашем кластере Kubernetes, то используемые образы ни при каких условиях не будут удалены при очистке.

При запуске очистки werf сканирует следующие типы объектов в кластере Kubernetes: `pod`, `deployment`, `replicaset`, `statefulset`, `daemonset`, `job`, `cronjob`, `replicationcontroller`.

Описанное поведение, — проверка объектов в кластере при очистке, может быть отключено параметром `--without-kube`.

##### Подключение к кластеру Kubernetes

werf получает информацию о кластерах Kubernetes и способах подключения к ним из файла конфигурации kubectl — `~/.kube/config`. Для сбора информации об используемых объектами образах, werf подключается **ко всем кластерам** Kubernetes, описанным **во всех контекстах** конфигурации kubectl.

### Очистка хранилища стадий

Выполнение очистки хранилища стадий с помощью команды [werf stages cleanup]({{ site.baseurl }}/documentation/cli/management/stages/cleanup.html) необходимо, чтобы синхронизировать его состояние с состоянием Docker registry.

Выполняя эту операцию, werf удаляет _стадии_, которые не связаны ни с одним образом в Docker registry.

> Если первый этап очистки по политикам, выполнение команды [werf images cleanup]({{ site.baseurl }}/documentation/cli/management/images/cleanup.html), был пропущен, то выполнение команды [werf stages cleanup]({{ site.baseurl }}/documentation/cli/management/stages/cleanup.html) не даст никакого эффекта

## Ручная очистка

Ручная очистка подразумевает полное удаление образов из _хранилища стадий_ или Docker registry (в зависимости от команды). Ручная очистка не учитывает, используется образ в кластере Kubernetes или нет.

Ручная очистка не подразумевает использования для запуска по расписанию. Она предназначена преимущественно для ручного удаления всех образов проекта.

Выполнение ручной очистки возможно следующими способами:
* Команда [werf images purge]({{ site.baseurl }}/documentation/cli/management/images/purge.html). Удаляет все образы **текущего проекта** в Docker registry.
* Команда [werf stages purge]({{ site.baseurl }}/documentation/cli/management/stages/purge.html). Удаляет все стадии **текущего проекта** в [хранилище стадий]({{ site.baseurl }}/documentation/reference/stages_and_images.html#хранилище-стадий).

Оба способа ручной очистки объединены в команде [werf purge]({{ site.baseurl }}/documentation/cli/main/purge.html), при которой сначала выполняется удаление образов проекта из Docker registry (_werf images purge_), а затем, удаление образов из _хранилища стадий_ _werf stages purge_).

## Очистка хоста

Для очистки хоста, на котором используется werf, предназначены следующие команды:

* [werf host cleanup]({{ site.baseurl }}/documentation/cli/management/host/cleanup.html). Очищает старые, неиспользуемые и неактуальные данные, включая кэш стадий во всех проектах на хосте.
* [werf host purge]({{ site.baseurl }}/documentation/cli/management/host/purge.html). Удаляет образы, стадии, кэш и другие данные (служебные папки, временные файлы), относящиеся к любому проекту werf на хосте. Другими словами, удаляет все следы werf для всех проектов. Эта команда обеспечивает максимальную степень очистки. Используйте её, например, если не планируете больше использовать werf на данном хосте.