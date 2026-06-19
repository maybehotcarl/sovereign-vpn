create table if not exists public.operator_enrollments (
  token text primary key,
  operator text not null,
  region text not null,
  status text not null default 'created'
    check (status in ('created', 'reported', 'healthy')),
  created_at timestamptz not null default now(),
  expires_at timestamptz not null,
  last_report_at timestamptz,
  report_operator text,
  report_region text,
  endpoint text,
  gateway_url text,
  public_ip text,
  gateway_port text,
  wireguard_port text,
  wireguard_public_key text,
  health_ok boolean not null default false,
  health_status text,
  installer_version text,
  reported_at timestamptz
);

create index if not exists operator_enrollments_operator_idx
  on public.operator_enrollments (operator);

create index if not exists operator_enrollments_expires_at_idx
  on public.operator_enrollments (expires_at);

create index if not exists operator_enrollments_status_idx
  on public.operator_enrollments (status);

alter table public.operator_enrollments enable row level security;

do $$
begin
  if exists (select 1 from pg_roles where rolname = 'anon') then
    revoke all on table public.operator_enrollments from anon;
  end if;

  if exists (select 1 from pg_roles where rolname = 'authenticated') then
    revoke all on table public.operator_enrollments from authenticated;
  end if;
end $$;
