# Documentation policy

## Self-linting

Run `make docs-lint` to check the published Markdown and README with Marklint built from this checkout. The default `make verify` and every CI runner execute this target.

The YAML policy in `docs/lint.yaml` composes the recommended Markdown rules and the STE100 and Strunk and White checks. It adds one title per page, repeated-word detection and the required rule-page headings. Generated rule pages include YAML frontmatter with their title and check ID. The rule index uses its own layout.

The template validates rule metadata before generation. It requires a behavior summary, parameter descriptions, YAML configuration options, failing and corrected examples, and an explanation. The [metadata schema](rule-reference.schema.yaml) describes this format. Go tests compile every configuration. Markdown tests also execute each documented example against its check, including fixtures for links and duplicate IDs.

## Approved vocabulary

`docs/vocabulary.yaml` contains the committed project vocabulary. It is a project-specific extension for developer documentation, instead of the official ASD dictionary. The initial vocabulary covers the existing documentation corpus. CI never derives words from the current text or automatically expands the dictionary.

Add a word only after checking its meaning and use. Group approved inflections under an entry with explicit `forms`. Listing a base word does not approve other forms. Part-of-speech and meaning fields document the decision; the current checker does not infer them from context.

The project sentence limit is 19 lexical words, which enforces fewer than 20. Code, URLs and frontmatter are excluded. Intentional failing examples belong in fenced code blocks. The same prose scope applies to manual guides and generated rule pages.

For an unapproved word, change the prose or propose a deliberate dictionary update. Run the self-lint and dictionary tests before committing. Tests inject an unknown word, an overlong sentence and a page-shape defect to prove that the policy rejects them.

## Configurable wording checks

Use `text.matcher` for banned words, phrases, characters and patterns. Portos dash and phrase policies are configured instances. Strunk and White names supply defaults to the same matcher. Their options remain independently configurable.
