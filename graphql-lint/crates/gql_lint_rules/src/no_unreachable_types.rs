//! Ports `internal/graphql/lint/unreachable_types.go`'s `no-unreachable-types`
//! rule: forbids a type or directive definition that no root operation
//! type's fields can ever reach.
//!
//! **One deliberate simplification from the Go port, not hidden:** the Go
//! version reports a base definition and each of its `extend type`
//! occurrences as separate violations sharing one name (`@graphql-eslint`'s
//! own test suite expects this), because it can walk `doc.Definitions` and
//! `doc.Extensions` separately even after `validator.ValidateSchemaDocument`
//! has merged them for the reachability computation itself. This port only
//! has `apollo_compiler::Schema` — the merged view, with no surfaced way
//! back to which of possibly several `extend type Foo { ... }` blocks (or
//! the base `type Foo { ... }`) an unreachable name came from. So this
//! reports one violation per unreachable *name*, not one per occurrence.
//! Revisit if per-occurrence reporting turns out to matter in practice —
//! it would need the pre-merge `SchemaDocument`/AST, not `Schema` alone.

use apollo_compiler::Schema;
use apollo_compiler::ast::DirectiveLocation;
use apollo_compiler::schema::{ExtendedType, Name};
use gql_lint_core::{Violation, rules};
use std::collections::HashSet;

/// The 8 directive locations `@graphql-eslint`'s own
/// `RequestDirectiveLocations` treats as "usable while executing a
/// request": a directive definition usable at one of these is reachable
/// on its own, without anything in the schema referring to it.
const REQUEST_DIRECTIVE_LOCATIONS: &[DirectiveLocation] = &[
    DirectiveLocation::Query,
    DirectiveLocation::Mutation,
    DirectiveLocation::Subscription,
    DirectiveLocation::Field,
    DirectiveLocation::FragmentDefinition,
    DirectiveLocation::FragmentSpread,
    DirectiveLocation::InlineFragment,
    DirectiveLocation::VariableDefinition,
];

/// Runs the `no-unreachable-types` rule over `schema`.
#[must_use]
pub fn no_unreachable_types(schema: &Schema) -> Vec<Violation> {
    let reachable = reachable_type_names(schema);

    let mut violations = Vec::new();
    for ty in gql_lint_core::in_scope_types(schema) {
        let (name, label) = match ty {
            ExtendedType::Scalar(t) => (&t.name, "Scalar type"),
            ExtendedType::Object(t) => (&t.name, "Object type"),
            ExtendedType::Interface(t) => (&t.name, "Interface type"),
            ExtendedType::Union(t) => (&t.name, "Union type"),
            ExtendedType::Enum(t) => (&t.name, "Enum type"),
            ExtendedType::InputObject(t) => (&t.name, "Input object type"),
        };
        if !reachable.contains(name.as_str()) {
            violations.push(unreachable_violation(schema, label, name));
        }
    }
    for directive in gql_lint_core::in_scope_directive_definitions(schema) {
        if !reachable.contains(directive.name.as_str()) {
            violations.push(unreachable_violation(schema, "Directive", &directive.name));
        }
    }

    violations
}

fn unreachable_violation(schema: &Schema, label: &str, name: &Name) -> Violation {
    let (file, line, column) = gql_lint_core::resolve_location(&schema.sources, name.location());
    Violation {
        file,
        line,
        column,
        message: format!("{label} `{name}` is unreachable."),
        rule: rules::NO_UNREACHABLE_TYPES,
    }
}

/// Ports `unreachable_types.go`'s `reachableTypeNames`: a breadth-first
/// walk seeded from `schema`'s root operation types, the schema
/// definition's own applied directives, and any directive definition
/// usable at a request-execution location, following every named type a
/// reached definition's fields, arguments, interfaces, union members, and
/// applied directives refer to.
///
/// An interface is a deliberate special case, carried over from
/// `@graphql-eslint` unchanged: reaching one reaches every known
/// implementation of it instead of the interface's own body, since
/// `graphql-js` resolves an interface-typed field through whichever
/// concrete object actually implements it.
fn reachable_type_names(schema: &Schema) -> HashSet<String> {
    let mut reached: HashSet<String> = HashSet::new();
    let mut queue: Vec<String> = Vec::new();
    seed_reachable_roots(schema, &mut queue, &mut reached);

    let implementers = schema.implementers_map();

    while let Some(name) = queue.pop() {
        let Some(ty) = schema.types.get(name.as_str()) else {
            // Not a type: either a directive definition (its arguments are
            // walked below) or a name with no definition at all (an
            // already-reported Tier 1 problem elsewhere).
            if let Some(dd) = schema.directive_definitions.get(name.as_str()) {
                for arg in &dd.arguments {
                    enqueue(arg.ty.inner_named_type().as_str(), &mut queue, &mut reached);
                    collect_directives(
                        arg.directives.iter().map(|d| &d.name),
                        &mut queue,
                        &mut reached,
                    );
                }
            }
            continue;
        };

        match ty {
            ExtendedType::Interface(_) => {
                if let Some(impls) = implementers.get(name.as_str()) {
                    for object in &impls.objects {
                        enqueue(object.as_str(), &mut queue, &mut reached);
                    }
                    for iface in &impls.interfaces {
                        enqueue(iface.as_str(), &mut queue, &mut reached);
                    }
                }
            }
            ExtendedType::Object(t) => {
                for iface in &t.implements_interfaces {
                    enqueue(iface.as_str(), &mut queue, &mut reached);
                }
                collect_directives(
                    t.directives.0.iter().map(|d| &d.name),
                    &mut queue,
                    &mut reached,
                );
                for field in t.fields.values() {
                    enqueue(
                        field.ty.inner_named_type().as_str(),
                        &mut queue,
                        &mut reached,
                    );
                    for arg in &field.arguments {
                        enqueue(arg.ty.inner_named_type().as_str(), &mut queue, &mut reached);
                        collect_directives(
                            arg.directives.iter().map(|d| &d.name),
                            &mut queue,
                            &mut reached,
                        );
                    }
                    collect_directives(
                        field.directives.0.iter().map(|d| &d.name),
                        &mut queue,
                        &mut reached,
                    );
                }
            }
            ExtendedType::Union(t) => {
                for member in &t.members {
                    enqueue(member.as_str(), &mut queue, &mut reached);
                }
            }
            ExtendedType::Enum(t) => {
                for value in t.values.values() {
                    collect_directives(
                        value.directives.0.iter().map(|d| &d.name),
                        &mut queue,
                        &mut reached,
                    );
                }
            }
            ExtendedType::InputObject(t) => {
                for field in t.fields.values() {
                    enqueue(
                        field.ty.inner_named_type().as_str(),
                        &mut queue,
                        &mut reached,
                    );
                    collect_directives(
                        field.directives.0.iter().map(|d| &d.name),
                        &mut queue,
                        &mut reached,
                    );
                }
            }
            ExtendedType::Scalar(_) => {}
        }
    }

    reached
}

/// Seeds `queue`/`reached` with `schema`'s root operation types, the schema
/// definition's own applied directives, and any directive definition
/// usable at a request-execution location — the starting points
/// [`reachable_type_names`]'s walk expands from. Split out on its own so
/// that function stays under `clippy::pedantic`'s line-count cap.
fn seed_reachable_roots(schema: &Schema, queue: &mut Vec<String>, reached: &mut HashSet<String>) {
    if let Some(name) = &schema.schema_definition.query {
        enqueue(name.as_str(), queue, reached);
    }
    if let Some(name) = &schema.schema_definition.mutation {
        enqueue(name.as_str(), queue, reached);
    }
    if let Some(name) = &schema.schema_definition.subscription {
        enqueue(name.as_str(), queue, reached);
    }
    for directive in &schema.schema_definition.directives.0 {
        enqueue(directive.name.as_str(), queue, reached);
    }
    for (name, dd) in &schema.directive_definitions {
        if dd
            .locations
            .iter()
            .any(|loc| REQUEST_DIRECTIVE_LOCATIONS.contains(loc))
        {
            // Marked reachable directly, not enqueued for a body walk: a
            // directive usable at a request-execution location needs
            // nothing in the schema to reference it, and `@graphql-eslint`
            // itself never walks its arguments for this case either.
            reached.insert(name.as_str().to_string());
        }
    }
}

/// Enqueues `name` for a body walk, unless it's already been reached.
fn enqueue(name: &str, queue: &mut Vec<String>, reached: &mut HashSet<String>) {
    if reached.insert(name.to_string()) {
        queue.push(name.to_string());
    }
}

/// Enqueues every directive name in `names` — accepts either
/// `schema::DirectiveList`'s or `ast::DirectiveList`'s item type, since a
/// field/type/enum-value's applied directives and an argument's applied
/// directives happen to use different (but equally iterable) list types
/// in `apollo-compiler`'s data model.
fn collect_directives<'a>(
    names: impl Iterator<Item = &'a Name>,
    queue: &mut Vec<String>,
    reached: &mut HashSet<String>,
) {
    for name in names {
        if reached.insert(name.as_str().to_string()) {
            queue.push(name.as_str().to_string());
        }
    }
}
