-- Tenfold schema. Every row belongs to a user, directly or through its
-- upload. The API connects as the database owner and filters by user_id in
-- every query.
--
-- Row level security is on with no policies. Supabase exposes the public
-- schema over its own REST API; with RLS on and no policy, that API returns
-- nothing, so the Go API is the only way in.

create table uploads (
    id                  uuid primary key,
    user_id             uuid not null,
    filename            text not null,
    source              text not null check (source in ('upload', 'sample')),
    sample_name         text,
    storage_key         text,
    size_bytes          bigint not null,
    status              text not null default 'queued',
    stage               text not null default 'uploaded',
    progress            int not null default 0,
    message             text not null default '',
    pipeline            jsonb not null default '[]',
    stop                jsonb,
    first_parse_error   jsonb,
    error               jsonb,
    risk_threshold      int not null,
    line_count          int,
    event_count         int,
    parse_error_count   int,
    anomaly_count       int,
    incident_count      int,
    user_count          int,
    host_count          int,
    time_start          timestamptz,
    time_end            timestamptz,
    created_at          timestamptz not null default now(),
    started_at          timestamptz,
    completed_at        timestamptz
);
create index uploads_user_created on uploads (user_id, created_at desc, id desc);

-- One row per parsed log line. The event ID in the API is 'e' || line_no.
create table events (
    upload_id    uuid not null references uploads (id) on delete cascade,
    line_no      int not null,
    ts           timestamptz not null,
    usr          text not null,
    dept         text,
    device       text,
    src_ip       text not null,
    host         text not null,
    url          text not null,
    category     text not null,
    dst_ip       text,
    method       text not null,
    status       int not null,
    bytes_out    bigint not null,
    bytes_in     bigint not null,
    user_agent   text,
    action       text not null check (action in ('allowed', 'blocked')),
    reason       text,
    threat_name  text,
    risk_score   int,
    raw          text not null,
    primary key (upload_id, line_no)
);
create index events_upload_ts on events (upload_id, ts, line_no);
create index events_upload_user on events (upload_id, usr);
create index events_upload_host on events (upload_id, host);

create table incidents (
    id              uuid primary key,
    upload_id       uuid not null references uploads (id) on delete cascade,
    user_id         uuid not null,
    title           text not null,
    severity        text not null,
    severity_rank   int not null,
    risk_score      int not null,
    ai_verdict      text,
    summary         text,
    marker_label    text not null,
    marker_detail   text,
    principal_user  text,
    principal_host  text,
    time_start      timestamptz not null,
    time_end        timestamptz not null,
    event_count     int not null,
    anomaly_count   int not null,
    mitre           jsonb not null default '[]',
    line_nos        int[] not null,
    -- Everything else the incident screen draws: narrative, signals,
    -- timeline, graph, indicators, chart, lesson suggestions.
    detail          jsonb not null,
    analyst_status  text not null default 'open',
    analyst_note    text,
    decided_at      timestamptz,
    assignee_id     uuid,
    assignee_name   text,
    created_at      timestamptz not null default now()
);
create index incidents_upload on incidents (upload_id);

create table anomalies (
    id                       uuid primary key,
    upload_id                uuid not null references uploads (id) on delete cascade,
    detector                 text not null,
    reason                   text not null,
    label                    text not null,
    signals                  jsonb not null,
    risk_score               int not null,
    decision                 jsonb,
    shadow_decision          jsonb,
    -- 'pending' while the job is still deciding; never returned by the API.
    disposition              text not null default 'pending',
    incident_id              uuid references incidents (id) on delete set null,
    usr                      text,
    device                   text,
    host                     text,
    hosts                    text[] not null default '{}',
    line_nos                 int[] not null,
    first_seen               timestamptz not null,
    last_seen                timestamptz not null,
    suppressed_by_lesson_id  uuid
);
create index anomalies_upload_score on anomalies (upload_id, risk_score desc, id);

-- Which events each anomaly rests on, for lookups from the event side.
create table anomaly_events (
    upload_id   uuid not null references uploads (id) on delete cascade,
    line_no     int not null,
    anomaly_id  uuid not null references anomalies (id) on delete cascade,
    primary key (upload_id, line_no, anomaly_id)
);

create table incident_notes (
    id           uuid primary key,
    incident_id  uuid not null references incidents (id) on delete cascade,
    author       text not null,
    text         text not null,
    created_at   timestamptz not null default now()
);
create index incident_notes_incident on incident_notes (incident_id, created_at);

create table lessons (
    id           uuid primary key,
    user_id      uuid not null,
    text         text not null,
    scope_type   text not null check (scope_type in ('user', 'domain', 'pattern')),
    scope_value  text not null,
    created_at   timestamptz not null default now(),
    expires_at   timestamptz not null,
    match_count  int not null default 0
);
create index lessons_user on lessons (user_id, created_at desc);

alter table uploads enable row level security;
alter table events enable row level security;
alter table incidents enable row level security;
alter table anomalies enable row level security;
alter table anomaly_events enable row level security;
alter table incident_notes enable row level security;
alter table lessons enable row level security;
