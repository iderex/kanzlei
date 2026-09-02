# Transparency: telling a user they are talking to a machine

A person asking this system a question is interacting with an AI system and is
reading text a model generated. There are transparency duties attached to both
of those facts in Union law. This document records which ones, by article, who
carries each of them, and what this project can and cannot do about them.

It is a reading of the text rather than a summary of what the obligation is
generally taken to be, because the generally taken version is wrong on the two
points that matter most here: which paragraph of the article reaches an
assistant like this one, and whether being open source changes the answer.

## What was read, and where

Regulation (EU) 2024/1689 of the European Parliament and of the Council of 13
June 2024 laying down harmonised rules on artificial intelligence, published in
the Official Journal as OJ L, 2024/1689, 12.7.2024, and known as the AI Act.
Its identifier for a reader who wants to check any quotation below is CELEX
32024R1689.

The wording quoted here was read on 2026-08-09 from a public reproduction of the
regulation at `artificialintelligenceact.eu`, article by article, and not from
the Official Journal itself. That is a weaker source than the Official Journal
and it is named rather than hidden: anybody relying on a sentence below should
read it again in the Official Journal text before relying on it. What is not
weaker is the article and paragraph numbering, which is what makes checking
cheap.

Union law only. No other jurisdiction was evaluated, so nothing here says
anything about a deployment outside the Union or about the duties a national law
adds on top.

## The duties are already in application

Article 113 sets the dates. The regulation entered into force on the twentieth
day after publication and, in its own words:

    It shall apply from 2 August 2026.

Article 50 sits in Chapter IV and takes that general date rather than one of the
earlier or later ones in the same article. So these are duties in application
now, not duties arriving later, and a deployment that starts today starts under
them.

## Article 50, paragraph by paragraph

### 50(1) reaches this system, and the duty is on the provider

    Providers shall ensure that AI systems intended to interact directly with
    natural persons are designed and developed in such a way that the natural
    persons concerned are informed that they are interacting with an AI system,
    unless this is obvious from the point of view of a natural person who is
    reasonably well-informed, observant and circumspect, taking into account the
    circumstances and the context of use.

An assistant a person asks questions of is an AI system intended to interact
directly with natural persons. The paragraph reaches it.

The carve-out is worth reading rather than leaning on. It is not satisfied by
the deploying organisation believing the fact is obvious, and it is not
satisfied by the system having a name that sounds like software. It is a test
about a reasonably well-informed, observant and circumspect person in the
circumstances and the context of use, and an internal search assistant that
answers in prose is a context where a user can reasonably take an answer for a
colleague's summary. The cheap and safe reading is that the disclosure is made.

### 50(2) also reaches it, and this is the paragraph that is usually missed

    Providers of AI systems, including general-purpose AI systems, generating
    synthetic audio, image, video or text content, shall ensure that the outputs
    of the AI system are marked in a machine-readable format and detectable as
    artificially generated or manipulated.

This is a second and larger obligation than the sentence a user sees. It is
about the output rather than about the interaction, it asks for a machine
readable marking, and it is owed by the provider.

The same paragraph carries an exception, and the exception is where a reading
could go wrong in the direction that costs something:

    This obligation shall not apply to the extent the AI systems perform an
    assistive function for standard editing or do not substantially alter the
    input data provided by the deployer or the semantics thereof.

An assistant that composes an answer out of passages it retrieved is not
performing standard editing, and the answer is not the deployer's input data
with its semantics preserved. So the exception is doubtful here and the
obligation is treated as reaching. Where a later version of this project returns
a passage verbatim and adds nothing, that specific output may sit inside the
exception, and that is a distinction to make per response rather than per
product.

The same paragraph also qualifies the duty by what is technically feasible and
by the state of the art as reflected in standards. That qualification is not a
way out of marking an output at all; it bears on how the marking is done.

### 50(3) does not reach it

    Deployers of an emotion recognition system or a biometric categorisation
    system shall inform the natural persons exposed thereto of the operation of
    the system.

This system is neither. It answers questions about documents and it categorises
nobody. What would change the answer is a feature that inferred something about
a person from biometric data, which the intended use note already says this
project is not built for.

### 50(4) does not reach the deployment, but it can reach an act of the deployer

The first subparagraph is about deep fakes, which this system does not produce.
The second is about text:

    Deployers of an AI system that generates or manipulates text which is
    published with the purpose of informing the public on matters of public
    interest shall disclose that the text has been artificially generated or
    manipulated.

An organisation asking questions of its own documents internally is not
publishing to inform the public. An organisation that takes an answer this
system produced and publishes it to inform the public on a matter of public
interest is inside this paragraph, and that is an act of theirs rather than a
property of the deployment. The same subparagraph excuses it where the content
went through human review or editorial control and a person or body holds
editorial responsibility for the publication.

### 50(5) fixes the manner and the moment

    The information referred to in paragraphs 1 to 4 shall be provided to the
    natural persons concerned in a clear and distinguishable manner at the
    latest at the time of the first interaction or exposure. The information
    shall conform to the applicable accessibility requirements.

Two things follow that a design can get wrong. The disclosure is owed at the
latest at the first interaction, so a sentence in a manual or in a terms page
somebody accepted once is not it. And it has to meet accessibility
requirements, which is a property of whatever surface a user meets.

### 50(6) and 50(7)

50(6) says paragraphs 1 to 4 leave the requirements of Chapter III untouched and
are without prejudice to other transparency obligations in Union or national
law. So nothing in this document is a ceiling.

50(7) is about codes of practice drawn up at Union level. It places no
obligation on a deployer of this system.

## Who carries the duty here

The two paragraphs that reach this system, 50(1) and 50(2), are addressed to
providers. That is the opposite of the assumption this work started from, which
was that the duty sits with the deployer and this project only helps.

Article 3 defines the roles. A provider is

    a natural or legal person, public authority, agency or other body that
    develops an AI system or a general-purpose AI model or that has an AI system
    or a general-purpose AI model developed and places it on the market or puts
    the AI system into service under its own name or trademark, whether for
    payment or free of charge

and a deployer is

    a natural or legal person, public authority, agency or other body using an
    AI system under its authority except where the AI system is used in the
    course of a personal non-professional activity.

Putting into service is defined in the same article as

    the supply of an AI system for first use directly to the deployer or for own
    use in the Union for its intended purpose

and making available on the market as

    the supply of an AI system or a general-purpose AI model for distribution or
    use on the Union market in the course of a commercial activity, whether in
    return for payment or free of charge.

Two readings follow, and they are readings rather than settled answers.

An organisation that takes this software and runs it on its own infrastructure
for its own people is supplying an AI system for own use in the Union for its
intended purpose. On the wording above that is putting into service, so that
organisation is likely to be a provider as well as a deployer, and the duties in
50(1) and 50(2) are then its own rather than somebody else's. An operator who
reads this document expecting to be only a deployer should stop at this
paragraph.

This repository, publishing source code, is not obviously making available on
the market, because that definition requires supply in the course of a
commercial activity. Nothing here is supplied commercially. That reading is not
a shield for the operator, because their own act of putting into service is
assessed separately from this one.

Neither reading has been tested against a national authority or a court, and
this document is not the place that settles them. What it does is name the
question early enough for an operator to take advice on it before deploying
rather than after.

## Whether being open source changes the answer

It does not, and the article says so by name.

    This Regulation does not apply to AI systems released under free and
    open-source licences, unless they are placed on the market or put into
    service as high-risk AI systems or as an AI system that falls under Article
    5 or 50.

Article 50 is written into the exception. So a system that falls under Article
50, which on the reading above this one does, is outside what the open-source
exclusion excludes. The exclusion is narrower than it is usually assumed to be
and this is the specific place where the assumption fails.

Recital 89 is the passage most often reached for on this point and it is about
something else:

    Third parties making accessible to the public tools, services, processes, or
    AI components other than general-purpose AI models, should not be mandated
    to comply with requirements targeting the responsibilities along the AI
    value chain, in particular towards the provider that has used or integrated
    them, when those tools, services, processes, or AI components are made
    accessible under a free and open-source licence.

That is about obligations along a value chain owed towards a provider who
integrated a component. It is not a transparency duty owed to the person reading
a generated answer, and it does not reach Article 50.

This paragraph carried a smaller point until 2026-09-02 and it has stopped
being true, which is worth stating rather than deleting. It said the exclusion
in Article 2(12) is about a system released under a free and open-source
licence, that this repository was released under none, and that the premise of
the exclusion was therefore not met here at all. A licence landed on
2026-08-17:

    gh api repos/iderex/kanzlei --jq .license.spdx_id
    "AGPL-3.0"

The GNU Affero General Public License is a free and open-source licence, so the
premise is met now. The direction that changes is the one that matters least
and the one that matters most at the same time. It changes nothing about the
conclusion above, because Article 50 is written into the exception by name and
a system falling under it is outside what the exclusion excludes whatever
licence it carries. It changes what the argument is doing: while the premise
failed, the whole section was moot here and could be read as an academic note.
It is load bearing now, and it is the paragraphs above this one rather than
this one that carry it.

## What this project does about it, and what it does not

Two of the four things this issue asks for are documents and are here. The other
two are behaviour and are not built.

Not built: a surface where a user meets the disclosure, and an answer that
carries the identifier of the model that produced it so a third-party client can
honour the same duty. There is no API package in this tree and no answer for
either to attach to. The model identifier those need is part of the capability
declaration in #71 and is recorded per call by #41. Whether this project ships
an interface of its own at all is an open question in #124, and the answer
changes who the first sentence of 50(1) is owed by: with no interface, every
surface a user meets belongs to somebody else.

50(2) is the one to carry forward into that work, because it is a design
obligation rather than a sentence. An answer has to be markable in a machine
readable form, which is a decision about the response shape in #78 rather than
something added to a user interface afterwards.

## What the deployer still has to do, which this project cannot do for them

Decide whether they are a provider as well as a deployer, on the reading above
and on advice this document does not give.

Write the disclosure a user sees, in the language their users read, and place it
where a user meets it at the latest at the first interaction rather than in a
policy they accepted once.

Meet the accessibility requirements that 50(5) attaches to that information, on
whatever surface they put in front of a user.

Disclose under 50(4) if they publish an answer to inform the public on a matter
of public interest, or hold the editorial responsibility that excuses it.

Keep whatever records other law requires of them. #98 is where the record of
processing an operator has to keep is dealt with, and nothing here replaces it.

## What is not evaluated here

Whether a particular deployment is a high-risk AI system under Chapter III and
Annex III. That is a question about what an organisation uses the answers for,
it is not answerable from this repository, and it is not answered here.

The obligations any national law adds, and every jurisdiction outside the Union.

What a national authority or a court would make of either reading in the section
on who carries the duty.

## This is not a compliance claim

Nothing in this document says that a deployment of this system complies with the
AI Act, and nothing here is legal advice.
