# Take Home Design.md

### Intro

This is my handwritten design doc.

I am building a POC where you log in, land on a screen that is large monitor friendly, that is ideal for a SOC analyst to stare at all day, as our primary user.

The app will take in Zscaler logs via a drag and drop upload field, with an alt button/link to “load demo data”.

The data file(s) will need to be stored somewhere. It will then need to be normalized and stored in a data store, in a human consumable form.

I am only going to use tech and code I am comfortable talking through. This will be reviewed with an architect from TENEX’s team.

This dashboard must include the ability to show an anomaly detection feature. Highlight anomalies found for the SOC analyst, with each one containing an explanation of why it was being flagged as one by AI, along with a confidence score.

The biggest challenge would be noise for an SOC. Pulling signal from the noise, in a performant and accurate manner.

Also, the ability for them to mark an anomaly as human verified or rejected, and adding reasons why, to train the AI further, in the future.

The only thing I know up front is that the “checks” and rules that events go through with anomaly dection will be multi-layered. I will work with AI to figure out the smartest layers to utilize.

I want to take a firehose of data, tame it, and product a handful of high quality anomalies from a dataset.

Below are my stack decisions, for this challenge.

---

## Chosen Stack

### Frontend

**Next.js**
I am going to go with Next.js for the framework, because I’m most familiar with it, I have server and client layers available, and an easy deployment pipeline up to Vercel, to save time.

**Basic Auth**
I am going to use Auth provided by Supabase. It’s quick to implement, robust, I’ve used it in production, and it’s directly tied to the Postgres db with RLS baked in. RLS will be on with no policies, which blocks direct client access; tenancy will be enforced in the Go API’s queries.

**File Uploads (logs)**
I will use a nice drag and drop library for the client side, and save the uploaded files using Supabase storage, to avoid setting up an S3 or GCS as a one-off. Same result. Less time spent.

**Display & Performance**
I will use libaries that make it easy to view the results. I prefer Chart.js, React Flow, and for Table or Grid, likely Tanstack because it offers virtualization. I care very much about performance and high FPS.

### Backend

I need to build a restful API. Supabase will store the db, and offer some layer of API support.

Postgres flows naturally with a REST api. If I need unstructured, I can also leverage Jsonb columns within this setup.

I will start here, but then if there is a comfortable amount of time, I will expand into using a Go backend framework. This is what TENEX.ai tends to use a lot, and they recommend, so I am happy to use it, even with a limited Go experience.

The most important piece for me, is to go spec-first with OpenAPI. I will write one openapi.yaml describing every endpoint and data shape. I can then both the Go side and Typescript side from the same file. One file, one contract, both languages. No drift.

Docs from the ground up.

### Database

Postgres on Supabase.

### Deployment/Hosting

Client = Vercel
API = Railway (likely, not committed yet)
DB = Supabase
Bonus = Dockerized combo on GCP

### AI

I will use AI to generate my code from this design doc.

I will also use it to power features in this POC including anomaly detection, etc.

My AI-first process for the project will be:

Session 1: Mockups to focus on SOC experience. Finalize.

Session 2: Generate open API doc, set up Vercel and Supabase projects.

Sessions 3 & 4 (Parallel): Generate frontend and backend from API spec. Separate repos in the same containing folder on my local machine, to simplify Git workflows given the time constraints of this exercise.

Session 5: Deep dive into log generation, and anomaly detection features, and implementation of all AI powered features.

Session 6: Hardening, Testing, Refining

Session 7: Packaging into one repo that runs with Docker. Final deployments.
