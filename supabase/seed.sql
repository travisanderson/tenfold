-- Seed for local development only. `npx supabase start` runs this once,
-- when it first creates the database. Production never sees it.
--
-- It creates what the app needs and the Go API does not create for itself:
-- one user to sign in as, and the bucket uploads go to. Both are public
-- knowledge and exist only on your machine.

-- The demo user: demo@local.test / demo-password
with demo as (
  insert into auth.users (
    instance_id, id, aud, role, email, encrypted_password, email_confirmed_at,
    raw_app_meta_data, raw_user_meta_data, created_at, updated_at,
    -- Auth reads these as text and fails on NULL.
    confirmation_token, recovery_token, email_change, email_change_token_new
  ) values (
    '00000000-0000-0000-0000-000000000000',
    '11111111-1111-4111-8111-111111111111',
    'authenticated', 'authenticated', 'demo@local.test',
    extensions.crypt('demo-password', extensions.gen_salt('bf')), now(),
    '{"provider": "email", "providers": ["email"]}', '{}', now(), now(),
    '', '', '', ''
  )
  on conflict (id) do nothing
  returning id, email
)
insert into auth.identities (id, user_id, provider_id, provider, identity_data, last_sign_in_at, created_at, updated_at)
select gen_random_uuid(), id, id::text, 'email',
       jsonb_build_object('sub', id::text, 'email', email, 'email_verified', true),
       now(), now(), now()
from demo;

-- The private bucket uploaded logs go to. 50 MB a file, as the API allows.
insert into storage.buckets (id, name, public, file_size_limit)
values ('logs', 'logs', false, 52428800)
on conflict (id) do nothing;
