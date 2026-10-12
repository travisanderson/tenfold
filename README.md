# Tenfold

Tenfold is a POC that tries to take the point of view that the SOC Analyst's life is harder than ever. Pulling signal from noise, which has now been cranked to a 10 because of AI threats, is super important.

I wanted to build something performant, with a solid modern stack, that imagines how to reduce alert fatigue for the user.

It reads a Zscaler log file, finds the odd behavior, and groups it into a handful of incidents, and explains each one with the evidence
beside it. The analyst decides from there.

## Delivered to you on a silver platter

Ready to go right now. Login with the provided credentials. Easy.\
[https://tenfold-ui.vercel.app](https://tenfold-ui.vercel.app)

You can simply click the load demo data button on the uploads page.

If you prefer to upload the log files yourself, here are a few:

- [attack-day.log](https://qftygikbzgytxrmfvrch.supabase.co/storage/v1/object/public/downloads/attack-day.log?download) (24 MB, 48,002 lines, four planted attacks)
- [clean-day.log](https://qftygikbzgytxrmfvrch.supabase.co/storage/v1/object/public/downloads/clean-day.log?download) (15 MB, an ordinary day)
- [large-100k.log](https://qftygikbzgytxrmfvrch.supabase.co/storage/v1/object/public/downloads/large-100k.log?download) (50 MB, 100,000 lines)

## Local Setup is stupid easy, as well.

1. `git clone https://github.com/travisanderson/tenfold.git && cd tenfold`
2. `npx supabase start`
3. `docker compose up`

Then open [http://localhost:3000](http://localhost:3000) and sign in as `demo@local.test` with the password `demo-password`. Gotta give it time to cook, though.

Out of the box it mocks AI use unless you populate `ANTHROPIC_API_KEY` in a `.env` file and set `AI_JUDGE_PROVIDER` and `AI_NARRATIVE_PROVIDER` to `claude`. See [.env.example](./.env.example).

Log files will be generated and placed into a Docker volume. To copy one out, run `docker compose cp samples:/generated/attack-day.log .`

---

## Original Design Doc (Human written)

I kicked off this process with this design doc that I wrote by hand, with clear thoughts step by step. Every fresh session with the agents, this was used as a starting point.\
[-> View Design Doc](./DESIGN.md)

## Stack Info (AI Generated)

This is where the stack and architecture ended up. I'm proud of it, pretty slick. See STACK.md to view more.\
[-> View Finished Stack](./explained/Stack.md)

One headline I will highlight here: I used one OpenAPI file, which generates the types for both sides - Go and Typescript. One file, one contract, both languages. No drift.

Jump right into the API Docs [here](https://tenfold-api.up.railway.app/docs#description/introduction).

## Walkthrough

-> You can view the walkthrough video [here](https://www.loom.com/embed/b6890c734c0c4ce1acccd4d3f4e00f02).\
..or...\
-> The written version is found [here](./explained/Walkthrough.md).

## Thank you.

Thanks for taking the time to take this out for a test drive. It's not perfect, there are small bugs, but given the time constraints, I'm satisfied with what I was able to deliver.

This is the best interview format for 2026, and it's much appreciated.
