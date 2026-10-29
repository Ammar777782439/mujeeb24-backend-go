# P1B — تدقيق الأنواع والقيود من PostgreSQL 16 بعد تطبيق جميع الترحيلات

**المرجع:** `Ammar777782439/mujeeb24-backend-go`، الفرع `feat/merchant-ai-assist` المشتق من `main`، دون دمج. **المصدر التشغيلي:** PostgreSQL 16 في GitHub Actions مع تشغيل `database.RunMigrations` لجميع ملفات `000001–000081`.

**مهم:** هذا تدقيق بنية *قاعدة اختبارية نظيفة*، وليس اتصالًا بقاعدة الإنتاج. ولا يُعد موافقة على إرسال أي عمود أو صف إلى AI. مصدر الحقيقة النهائي لهذه القراءة هو ملف `internal/platform/database/testdata/merchant_assist_schema_pg16.json`، ويقارنه اختبار Go مع مخطط قاعدة جديدة بعد تطبيق الترحيلات.

## 1. نتائج التحقق الحقيقي

| مقياس | نتيجة PostgreSQL الاختبارية |
|---|---:|
| الترحيلات المسجلة والمطبقة | 81 |
| الجداول النشطة (دون دفتر الترحيلات) | 77 |
| دفتر الترحيلات | 1 |
| جميع الأعمدة | 978 |
| قيود `PRIMARY KEY / UNIQUE / FOREIGN KEY / CHECK` | 676 |
| فهارس UNIQUE غير المرتبطة بـ`pg_constraint` | 25 |
| المصادر المرشحة لإسقاط آمن فقط | 33 |
| أعمدة المصادر المرشحة، جميعها **غير معتمدة بعد** | 425 |
| قيود تلك المصادر | 339 |

حُفظت الأنواع بواسطة `pg_catalog.format_type`، و`NOT NULL`، والقيمة الافتراضية، و`identity/generated`، والقيود بواسطة `pg_get_constraintdef`، والفهارس الفريدة المستقلة بواسطة `pg_get_indexdef`. أي اختلاف في صورة المخطط بعد إعادة تنفيذ الترحيلات يفشل اختبار CI، بدل تعديل المرجع آليًا.

## 2. لماذا لا نعتمد ملف schema.sql وحده؟

يحوي `schema.sql` تعريفات لـ46 جدولًا؛ أما PostgreSQL النهائي فيحوي 77 جدولًا نشطًا. توجد 32 جداول نشطة غير ممثلة فيه، إلى جانب اختلافات أعمدة في 8 جداول مشتركة. لم ندمجها بالتخمين أو نعدل `schema.sql` في هذه المرحلة.

**الجداول النشطة الغائبة عن schema.sql:** `ai_catalog_batches`، `ai_configuration_versions`، `ai_gemini_interactions`، `ai_provider_credentials`، `ai_provider_models`، `ai_provider_pricing_versions`، `ai_run_attempts`، `ai_runs`، `ai_stage_latencies`، `ai_tool_calls`، `ai_usage_records`، `ai_usage_telemetry`، `channel_history_purges`، `channel_provider_brands`، `merchant_ai_messages`، `merchant_ai_proposals`، `merchant_ai_sessions`، `plans`، `platform_audit_events`، `sales_action_jobs`، `sales_command_results`، `sales_confirmation_requests`، `sales_transaction_requirements`، `sales_workflows`، `subscription_addon_payments`، `subscription_addons`، `subscription_ai_usage`، `subscription_channel_pass_capacity_approvals`، `subscription_payments`، `subscriptions`، `support_messages`، `support_tickets`.

**اختلافات الأعمدة المرصودة بين snapshot النهائي وschema.sql:**

| الجدول | أعمدة أُضيفت/ظهرت في PostgreSQL | أعمدة كانت في schema.sql ولم تعد موجودة |
|---|---|---|
| `ai_decisions` | `ai_run_id`، `effective_action`، `effective_decision_reason`، `authorized_at`، `executed_at`، `requested_capability`، `effective_capability` | — |
| `attribute_definitions` | — | `is_searchable` |
| `automation_rules` | `position` | — |
| `business_policies` | `sales_workflow_enabled`، `allow_sales_interactions` | — |
| `commercial_transactions` | `agreement_version`، `transaction_data` | — |
| `conversation_state` | `summary`، `summary_turn_count`، `summary_last_message_id` | — |
| `conversations` | `last_gemini_interaction_id` | — |
| `team_invitations` | `display_name` | — |

## 3. جرد الأعمدة الفعلية للمصادر المرشحة (غير مصرح للنموذج بقراءتها)

**التصنيف `projection_candidate` لا يعرّف `AllowedFields` ولا يمنح الصلاحية.** لكل حقل أدناه، يجب أن توافق خدمة المجال على الغرض والتنقيح والصلاحية قبل تسجيل أي `ProjectionDescriptor`.


### `businesses` — business · ترحيل `000001`

الأعمدة: **11** · القيود: **9** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `name` | `text` | نعم | — |
| `slug` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `vertical_type` | `text` | نعم | — |
| `timezone` | `text` | نعم | — |
| `default_currency` | `character(3)` | نعم | — |
| `locale` | `text` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `businesses_currency_chk` (c)، `businesses_locale_not_blank_chk` (c)، `businesses_name_not_blank_chk` (c)، `businesses_pkey` (p)، `businesses_resource_version_positive_chk` (c)، `businesses_slug_not_blank_chk` (c)، `businesses_slug_uq` (u)، `businesses_status_chk` (c)، `businesses_vertical_type_chk` (c).


### `business_policies` — policy · ترحيل `000002`

الأعمدة: **14** · القيود: **4** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `business_id` | `uuid` | نعم | — |
| `ai_mode` | `text` | نعم | — |
| `default_human_review` | `boolean` | نعم | — |
| `allow_auto_reply` | `boolean` | نعم | — |
| `allow_auto_lead_creation` | `boolean` | نعم | — |
| `allow_auto_transaction_draft` | `boolean` | نعم | — |
| `allow_auto_confirmation` | `boolean` | نعم | — |
| `business_hours_reference` | `text` | لا | — |
| `escalation_policy_reference` | `text` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |
| `sales_workflow_enabled` | `boolean` | نعم | `false` |
| `allow_sales_interactions` | `boolean` | نعم | `false` |

**القيود المثبتة:** `business_policies_ai_mode_chk` (c)، `business_policies_business_fk` (f)، `business_policies_pkey` (p)، `business_policies_resource_version_positive_chk` (c).


### `channel_connections` — channels · ترحيل `000003`

الأعمدة: **12** · القيود: **10** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `provider_ref` | `text` | نعم | — |
| `channel` | `text` | نعم | — |
| `provider_account_ref` | `text` | لا | — |
| `provider_connection_ref` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `secret_reference` | `text` | نعم | — |
| `last_health_check_at` | `timestamp with time zone` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `channel_connections_business_fk` (f)، `channel_connections_business_id_uq` (u)، `channel_connections_channel_chk` (c)، `channel_connections_connection_ref_chk` (c)، `channel_connections_pkey` (p)، `channel_connections_provider_connection_uq` (u)، `channel_connections_provider_ref_chk` (c)، `channel_connections_resource_version_positive_chk` (c)، `channel_connections_secret_ref_chk` (c)، `channel_connections_status_chk` (c). **فهارس UNIQUE مستقلة:** `uq_channel_connections_provider_account`.


### `channel_connection_capabilities` — channels · ترحيل `000004`

الأعمدة: **5** · القيود: **4** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `connection_id` | `uuid` | نعم | — |
| `capability` | `text` | نعم | — |
| `enabled` | `boolean` | نعم | — |
| `checked_at` | `timestamp with time zone` | نعم | — |
| `evidence_source` | `text` | لا | — |

**القيود المثبتة:** `channel_connection_capabilities_capability_chk` (c)، `channel_connection_capabilities_connection_fk` (f)، `channel_connection_capabilities_evidence_chk` (c)، `channel_connection_capabilities_pk` (p).


### `customers` — customer · ترحيل `000005`

الأعمدة: **10** · القيود: **9** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `profile` | `jsonb` | نعم | `'{}'::jsonb` |
| `contact_points` | `jsonb` | نعم | `'[]'::jsonb` |
| `locale_preference` | `text` | لا | — |
| `status` | `text` | نعم | — |
| `merged_into_customer_id` | `uuid` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `customers_business_fk` (f)، `customers_business_id_uq` (u)، `customers_contact_points_array_chk` (c)، `customers_merge_same_business_fk` (f)، `customers_merged_target_chk` (c)، `customers_pkey` (p)، `customers_profile_object_chk` (c)، `customers_resource_version_positive_chk` (c)، `customers_status_chk` (c).


### `conversations` — inbox · ترحيل `000008`

الأعمدة: **13** · القيود: **10** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `customer_id` | `uuid` | نعم | — |
| `state` | `text` | نعم | — |
| `ownership` | `text` | نعم | — |
| `ai_mode_override` | `text` | لا | — |
| `priority` | `text` | نعم | — |
| `assignment_reference` | `text` | لا | — |
| `last_activity_at` | `timestamp with time zone` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |
| `last_gemini_interaction_id` | `text` | لا | — |

**القيود المثبتة:** `conversations_ai_mode_chk` (c)، `conversations_assignment_ref_chk` (c)، `conversations_business_fk` (f)، `conversations_business_id_uq` (u)، `conversations_customer_fk` (f)، `conversations_ownership_chk` (c)، `conversations_pkey` (p)، `conversations_priority_chk` (c)، `conversations_resource_version_positive_chk` (c)، `conversations_state_chk` (c).


### `catalogs` — catalog · ترحيل `000013`

الأعمدة: **8** · القيود: **6** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `name` | `text` | نعم | — |
| `description` | `text` | لا | — |
| `status` | `text` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `catalogs_business_fk` (f)، `catalogs_business_id_uq` (u)، `catalogs_name_chk` (c)، `catalogs_pkey` (p)، `catalogs_resource_version_chk` (c)، `catalogs_status_chk` (c).


### `attribute_schemas` — catalog · ترحيل `000014`

الأعمدة: **6** · القيود: **6** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `name` | `text` | نعم | — |
| `version` | `integer` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `attribute_schemas_business_fk` (f)، `attribute_schemas_business_id_uq` (u)، `attribute_schemas_name_chk` (c)، `attribute_schemas_pkey` (p)، `attribute_schemas_version_chk` (c)، `attribute_schemas_version_uq` (u).


### `attribute_definitions` — catalog · ترحيل `000015`

الأعمدة: **10** · القيود: **8** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `schema_id` | `uuid` | نعم | — |
| `attribute_key` | `text` | نعم | — |
| `label` | `text` | نعم | — |
| `data_type` | `text` | نعم | — |
| `is_required` | `boolean` | نعم | — |
| `validation_rules` | `jsonb` | نعم | `'{}'::jsonb` |
| `display_order` | `integer` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `attribute_definitions_key_chk` (c)، `attribute_definitions_key_uq` (u)، `attribute_definitions_label_chk` (c)، `attribute_definitions_order_chk` (c)، `attribute_definitions_pkey` (p)، `attribute_definitions_schema_fk` (f)، `attribute_definitions_type_chk` (c)، `attribute_definitions_validation_object_chk` (c).


### `catalog_items` — catalog · ترحيل `000016`

الأعمدة: **18** · القيود: **13** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `catalog_id` | `uuid` | نعم | — |
| `attribute_schema_id` | `uuid` | لا | — |
| `attribute_schema_version` | `integer` | لا | — |
| `item_type` | `text` | نعم | — |
| `name` | `text` | نعم | — |
| `short_description` | `text` | لا | — |
| `long_description` | `text` | لا | — |
| `status` | `text` | نعم | — |
| `pricing_mode` | `text` | نعم | — |
| `availability_mode` | `text` | نعم | — |
| `fulfillment_mode` | `text` | نعم | — |
| `requires_confirmation` | `boolean` | نعم | — |
| `attributes` | `jsonb` | نعم | `'{}'::jsonb` |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `catalog_items_attribute_schema_fk` (f)، `catalog_items_attributes_object_chk` (c)، `catalog_items_availability_mode_chk` (c)، `catalog_items_business_id_uq` (u)، `catalog_items_catalog_fk` (f)، `catalog_items_fulfillment_mode_chk` (c)، `catalog_items_name_chk` (c)، `catalog_items_pkey` (p)، `catalog_items_pricing_mode_chk` (c)، `catalog_items_resource_version_chk` (c)، `catalog_items_schema_version_pair_chk` (c)، `catalog_items_status_chk` (c)، `catalog_items_type_chk` (c).


### `variants` — catalog · ترحيل `000017`

الأعمدة: **9** · القيود: **8** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `catalog_item_id` | `uuid` | نعم | — |
| `name` | `text` | نعم | — |
| `attributes` | `jsonb` | نعم | — |
| `status` | `text` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `variants_attributes_object_chk` (c)، `variants_business_id_uq` (u)، `variants_business_item_id_uq` (u)، `variants_catalog_item_fk` (f)، `variants_name_chk` (c)، `variants_pkey` (p)، `variants_resource_version_chk` (c)، `variants_status_chk` (c).


### `offers` — catalog · ترحيل `000018`

الأعمدة: **25** · القيود: **18** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `catalog_item_id` | `uuid` | نعم | — |
| `variant_id` | `uuid` | لا | — |
| `name` | `text` | نعم | — |
| `pricing_mode` | `text` | نعم | — |
| `amount` | `numeric(20,4)` | لا | — |
| `currency` | `character(3)` | لا | — |
| `pricing_unit` | `text` | لا | — |
| `price_source` | `text` | لا | — |
| `price_verification_status` | `text` | نعم | — |
| `price_checked_at` | `timestamp with time zone` | لا | — |
| `availability_mode` | `text` | نعم | — |
| `availability_status` | `text` | نعم | — |
| `availability_source` | `text` | لا | — |
| `availability_checked_at` | `timestamp with time zone` | لا | — |
| `availability_valid_until` | `timestamp with time zone` | لا | — |
| `availability_evidence_ref` | `text` | لا | — |
| `fulfillment_mode` | `text` | نعم | — |
| `validity_from` | `timestamp with time zone` | لا | — |
| `validity_until` | `timestamp with time zone` | لا | — |
| `status` | `text` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `offers_amount_chk` (c)، `offers_availability_mode_chk` (c)، `offers_availability_status_chk` (c)، `offers_availability_validity_chk` (c)، `offers_business_id_uq` (u)، `offers_business_item_id_uq` (u)، `offers_catalog_item_fk` (f)، `offers_currency_chk` (c)، `offers_fulfillment_mode_chk` (c)، `offers_name_chk` (c)، `offers_pkey` (p)، `offers_price_verification_chk` (c)، `offers_pricing_consistency_chk` (c)، `offers_pricing_mode_chk` (c)، `offers_resource_version_chk` (c)، `offers_status_chk` (c)، `offers_validity_chk` (c)، `offers_variant_same_item_fk` (f).


### `leads` — sales · ترحيل `000019`

الأعمدة: **23** · القيود: **19** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `customer_id` | `uuid` | نعم | — |
| `status` | `text` | نعم | — |
| `current_score_value` | `numeric(5,2)` | لا | — |
| `current_score_band` | `text` | لا | — |
| `score_rule_version` | `text` | لا | — |
| `score_model_reference` | `text` | لا | — |
| `qualification_context` | `jsonb` | نعم | `'{}'::jsonb` |
| `created_by` | `text` | نعم | — |
| `qualified_by` | `text` | لا | — |
| `lost_reason` | `text` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `source_conversation_reference_id` | `uuid` | لا | — |
| `source_channel` | `text` | لا | — |
| `intent_reference` | `text` | لا | — |
| `assigned_ownership_reference` | `text` | لا | — |
| `next_action_at` | `timestamp with time zone` | لا | — |
| `qualification_reason` | `text` | لا | — |
| `qualification_evidence` | `jsonb` | نعم | `'[]'::jsonb` |
| `qualification_state` | `text` | نعم | `'new'::text` |
| `resource_version` | `bigint` | نعم | `1` |

**القيود المثبتة:** `leads_assignment_reference_chk` (c)، `leads_business_fk` (f)، `leads_business_id_uq` (u)، `leads_context_object_chk` (c)، `leads_created_by_chk` (c)، `leads_customer_fk` (f)، `leads_intent_reference_chk` (c)، `leads_lost_reason_chk` (c)، `leads_pkey` (p)، `leads_qualification_evidence_array_chk` (c)، `leads_qualification_reason_chk` (c)، `leads_qualification_state_chk` (c)، `leads_qualified_by_chk` (c)، `leads_resource_version_chk` (c)، `leads_score_band_chk` (c)، `leads_score_value_chk` (c)، `leads_source_channel_chk` (c)، `leads_source_conversation_reference_fk` (f)، `leads_status_chk` (c).


### `lead_attributions` — sales · ترحيل `000020`

الأعمدة: **10** · القيود: **7** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `lead_id` | `uuid` | نعم | — |
| `source_conversation_id` | `uuid` | لا | — |
| `source_channel` | `text` | لا | — |
| `source_interaction_reference` | `text` | لا | — |
| `catalog_item_id` | `uuid` | لا | — |
| `offer_id` | `uuid` | لا | — |
| `campaign_reference` | `text` | لا | — |
| `captured_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `lead_attributions_business_fk` (f)، `lead_attributions_catalog_item_fk` (f)، `lead_attributions_conversation_fk` (f)، `lead_attributions_lead_fk` (f)، `lead_attributions_offer_fk` (f)، `lead_attributions_pkey` (p)، `lead_attributions_source_channel_chk` (c).


### `lead_scores` — sales · ترحيل `000021`

الأعمدة: **10** · القيود: **6** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `lead_id` | `uuid` | نعم | — |
| `value` | `numeric(5,2)` | نعم | — |
| `band` | `text` | نعم | — |
| `factors` | `jsonb` | نعم | — |
| `rule_version` | `text` | لا | — |
| `model_reference` | `text` | لا | — |
| `calculated_at` | `timestamp with time zone` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `lead_scores_band_chk` (c)، `lead_scores_business_fk` (f)، `lead_scores_factors_object_chk` (c)، `lead_scores_lead_fk` (f)، `lead_scores_pkey` (p)، `lead_scores_value_chk` (c).


### `commercial_transactions` — sales · ترحيل `000022`

الأعمدة: **17** · القيود: **16** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `customer_id` | `uuid` | نعم | — |
| `lead_id` | `uuid` | لا | — |
| `transaction_type` | `text` | نعم | — |
| `state` | `text` | نعم | — |
| `source_conversation_reference_id` | `uuid` | لا | — |
| `currency` | `character(3)` | لا | — |
| `total_amount` | `numeric(20,4)` | لا | — |
| `schema_version` | `integer` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `requires_human_review` | `boolean` | نعم | `false` |
| `cancellation_reason` | `text` | لا | — |
| `resource_version` | `bigint` | نعم | `1` |
| `agreement_version` | `bigint` | نعم | `1` |
| `transaction_data` | `jsonb` | نعم | `'{}'::jsonb` |

**القيود المثبتة:** `commercial_transactions_agreement_version_check` (c)، `commercial_transactions_amount_chk` (c)، `commercial_transactions_amount_currency_chk` (c)، `commercial_transactions_business_fk` (f)، `commercial_transactions_business_id_uq` (u)، `commercial_transactions_cancellation_reason_chk` (c)، `commercial_transactions_conversation_reference_fk` (f)، `commercial_transactions_currency_chk` (c)، `commercial_transactions_customer_fk` (f)، `commercial_transactions_lead_fk` (f)، `commercial_transactions_pkey` (p)، `commercial_transactions_resource_version_chk` (c)، `commercial_transactions_schema_version_chk` (c)، `commercial_transactions_state_chk` (c)، `commercial_transactions_transaction_data_check` (c)، `commercial_transactions_type_chk` (c).


### `transaction_confirmations` — sales · ترحيل `000023`

الأعمدة: **10** · القيود: **7** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `transaction_id` | `uuid` | نعم | — |
| `status` | `text` | نعم | — |
| `confirmed_by` | `text` | لا | — |
| `confirmed_at` | `timestamp with time zone` | لا | — |
| `evidence_reference` | `text` | لا | — |
| `policy_version` | `text` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `transaction_confirmations_business_fk` (f)، `transaction_confirmations_confirmed_by_chk` (c)، `transaction_confirmations_lifecycle_chk` (c)، `transaction_confirmations_pkey` (p)، `transaction_confirmations_status_chk` (c)، `transaction_confirmations_transaction_fk` (f)، `transaction_confirmations_transaction_uq` (u).


### `transaction_reviews` — sales · ترحيل `000024`

الأعمدة: **11** · القيود: **9** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `transaction_id` | `uuid` | نعم | — |
| `required` | `boolean` | نعم | — |
| `status` | `text` | نعم | — |
| `reason_codes` | `jsonb` | نعم | `'[]'::jsonb` |
| `reviewer_reference` | `text` | لا | — |
| `decided_at` | `timestamp with time zone` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `decision_reason` | `text` | لا | — |

**القيود المثبتة:** `transaction_reviews_business_fk` (f)، `transaction_reviews_decision_reason_chk` (c)، `transaction_reviews_lifecycle_chk` (c)، `transaction_reviews_pkey` (p)، `transaction_reviews_reason_codes_array_chk` (c)، `transaction_reviews_reviewer_chk` (c)، `transaction_reviews_status_chk` (c)، `transaction_reviews_transaction_fk` (f)، `transaction_reviews_transaction_uq` (u).


### `order_lines` — sales · ترحيل `000025`

الأعمدة: **17** · القيود: **17** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `transaction_id` | `uuid` | نعم | — |
| `catalog_item_id` | `uuid` | نعم | — |
| `offer_id` | `uuid` | لا | — |
| `variant_id` | `uuid` | لا | — |
| `item_name_snapshot` | `text` | نعم | — |
| `selected_attributes_snapshot` | `jsonb` | نعم | `'{}'::jsonb` |
| `quantity` | `numeric(20,4)` | نعم | — |
| `unit_price_snapshot` | `numeric(20,4)` | لا | — |
| `line_total_snapshot` | `numeric(20,4)` | لا | — |
| `currency` | `character(3)` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `pricing_snapshot` | `jsonb` | نعم | `'{}'::jsonb` |
| `availability_snapshot` | `jsonb` | نعم | `'{}'::jsonb` |
| `fulfillment_snapshot` | `jsonb` | نعم | `'{}'::jsonb` |

**القيود المثبتة:** `order_lines_attributes_object_chk` (c)، `order_lines_availability_snapshot_object_chk` (c)، `order_lines_business_fk` (f)، `order_lines_business_id_uq` (u)، `order_lines_catalog_item_fk` (f)، `order_lines_currency_chk` (c)، `order_lines_fulfillment_snapshot_object_chk` (c)، `order_lines_item_name_chk` (c)، `order_lines_offer_same_item_fk` (f)، `order_lines_pkey` (p)، `order_lines_price_currency_chk` (c)، `order_lines_pricing_snapshot_object_chk` (c)، `order_lines_quantity_chk` (c)، `order_lines_total_price_chk` (c)، `order_lines_transaction_fk` (f)، `order_lines_unit_price_chk` (c)، `order_lines_variant_same_item_fk` (f).


### `communication_messages` — inbox · ترحيل `000028`

الأعمدة: **15** · القيود: **14** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `conversation_reference_id` | `uuid` | نعم | — |
| `inbound_event_id` | `uuid` | لا | — |
| `outbound_message_id` | `uuid` | لا | — |
| `direction` | `text` | نعم | — |
| `origin` | `text` | نعم | — |
| `transport` | `text` | نعم | — |
| `provider_message_id` | `text` | لا | — |
| `content_type` | `text` | نعم | — |
| `text_content` | `text` | لا | — |
| `content_reference` | `text` | نعم | — |
| `occurred_at` | `timestamp with time zone` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `visibility` | `text` | نعم | `'public'::text` |

**القيود المثبتة:** `communication_messages_business_fk` (f)، `communication_messages_business_id_uq` (u)، `communication_messages_content_reference_chk` (c)، `communication_messages_content_type_chk` (c)، `communication_messages_conversation_reference_fk` (f)، `communication_messages_direction_chk` (c)، `communication_messages_external_reference_chk` (c)، `communication_messages_inbound_event_fk` (f)، `communication_messages_origin_chk` (c)، `communication_messages_outbound_message_fk` (f)، `communication_messages_pkey` (p)، `communication_messages_text_content_chk` (c)، `communication_messages_transport_chk` (c)، `communication_messages_visibility_chk` (c). **فهارس UNIQUE مستقلة:** `uq_communication_messages_provider_message`.


### `business_policy_versions` — knowledge · ترحيل `000036`

الأعمدة: **14** · القيود: **13** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `policy_key` | `text` | نعم | — |
| `category` | `text` | نعم | — |
| `title` | `text` | نعم | — |
| `summary` | `text` | نعم | — |
| `rules` | `jsonb` | نعم | — |
| `authority` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `version` | `integer` | نعم | — |
| `valid_from` | `timestamp with time zone` | نعم | — |
| `valid_until` | `timestamp with time zone` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `business_policy_versions_authority_chk` (c)، `business_policy_versions_business_fk` (f)، `business_policy_versions_business_id_uq` (u)، `business_policy_versions_category_chk` (c)، `business_policy_versions_key_chk` (c)، `business_policy_versions_key_version_uq` (u)، `business_policy_versions_pkey` (p)، `business_policy_versions_rules_object_chk` (c)، `business_policy_versions_status_chk` (c)، `business_policy_versions_summary_chk` (c)، `business_policy_versions_title_chk` (c)، `business_policy_versions_validity_chk` (c)، `business_policy_versions_version_chk` (c). **فهارس UNIQUE مستقلة:** `uq_business_policy_versions_published_key`.


### `knowledge_documents` — knowledge · ترحيل `000036`

الأعمدة: **14** · القيود: **13** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `knowledge_key` | `text` | نعم | — |
| `title` | `text` | نعم | — |
| `content` | `text` | نعم | — |
| `content_type` | `text` | نعم | — |
| `source_reference` | `text` | نعم | — |
| `authority` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `version` | `integer` | نعم | — |
| `valid_from` | `timestamp with time zone` | نعم | — |
| `valid_until` | `timestamp with time zone` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `knowledge_documents_authority_chk` (c)، `knowledge_documents_business_fk` (f)، `knowledge_documents_business_id_uq` (u)، `knowledge_documents_content_chk` (c)، `knowledge_documents_content_type_chk` (c)، `knowledge_documents_key_chk` (c)، `knowledge_documents_key_version_uq` (u)، `knowledge_documents_pkey` (p)، `knowledge_documents_source_chk` (c)، `knowledge_documents_status_chk` (c)، `knowledge_documents_title_chk` (c)، `knowledge_documents_validity_chk` (c)، `knowledge_documents_version_chk` (c). **فهارس UNIQUE مستقلة:** `uq_knowledge_documents_published_key`.


### `conversation_labels` — inbox · ترحيل `000044`

الأعمدة: **4** · القيود: **5** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `business_id` | `uuid` | نعم | — |
| `conversation_id` | `uuid` | نعم | — |
| `label` | `text` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `conversation_labels_business_fk` (f)، `conversation_labels_conversation_fk` (f)، `conversation_labels_normalized_chk` (c)، `conversation_labels_not_blank_chk` (c)، `conversation_labels_pkey` (p).


### `canned_replies` — inbox · ترحيل `000047`

الأعمدة: **9** · القيود: **8** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `title` | `text` | نعم | — |
| `shortcut` | `text` | نعم | — |
| `body` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `canned_replies_body_not_blank_chk` (c)، `canned_replies_business_fk` (f)، `canned_replies_pkey` (p)، `canned_replies_resource_version_chk` (c)، `canned_replies_shortcut_normalized_chk` (c)، `canned_replies_shortcut_not_blank_chk` (c)، `canned_replies_status_chk` (c)، `canned_replies_title_not_blank_chk` (c). **فهارس UNIQUE مستقلة:** `canned_replies_business_shortcut_uq`.


### `automation_rules` — automation · ترحيل `000048`

الأعمدة: **12** · القيود: **10** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `name` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `trigger_kind` | `text` | نعم | — |
| `conditions` | `jsonb` | نعم | `'{}'::jsonb` |
| `action_kind` | `text` | نعم | — |
| `action_payload` | `jsonb` | نعم | `'{}'::jsonb` |
| `position` | `integer` | نعم | `100` |
| `resource_version` | `bigint` | نعم | `1` |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `automation_rules_action_kind_chk` (c)، `automation_rules_action_payload_object_chk` (c)، `automation_rules_business_fk` (f)، `automation_rules_conditions_object_chk` (c)، `automation_rules_name_not_blank_chk` (c)، `automation_rules_pkey` (p)، `automation_rules_position_positive_chk` (c)، `automation_rules_resource_version_chk` (c)، `automation_rules_status_chk` (c)، `automation_rules_trigger_kind_chk` (c).


### `conversation_state` — inbox · ترحيل `000052`

الأعمدة: **14** · القيود: **11** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `business_id` | `uuid` | نعم | — |
| `conversation_id` | `uuid` | نعم | — |
| `focus` | `jsonb` | لا | — |
| `previous` | `jsonb` | نعم | `'[]'::jsonb` |
| `comparison` | `jsonb` | لا | — |
| `preferences` | `jsonb` | نعم | `'[]'::jsonb` |
| `constraints` | `jsonb` | نعم | `'[]'::jsonb` |
| `pending` | `jsonb` | نعم | `'[]'::jsonb` |
| `version` | `bigint` | نعم | `1` |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `summary` | `text` | لا | — |
| `summary_turn_count` | `integer` | نعم | `0` |
| `summary_last_message_id` | `uuid` | لا | — |

**القيود المثبتة:** `conversation_state_business_fk` (f)، `conversation_state_comparison_object_chk` (c)، `conversation_state_constraints_array_chk` (c)، `conversation_state_conversation_fk` (f)، `conversation_state_focus_object_chk` (c)، `conversation_state_pending_array_chk` (c)، `conversation_state_pkey` (p)، `conversation_state_preferences_array_chk` (c)، `conversation_state_previous_array_chk` (c)، `conversation_state_summary_turn_count_check` (c)، `conversation_state_version_chk` (c).


### `plans` — billing · ترحيل `000061`

الأعمدة: **17** · القيود: **15** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `code` | `text` | نعم | — |
| `version` | `integer` | نعم | `1` |
| `display_name` | `text` | نعم | — |
| `price_yer` | `integer` | نعم | — |
| `billing_interval` | `text` | نعم | `'MONTH'::text` |
| `ai_reply_limit` | `integer` | نعم | — |
| `ai_catalog_limit` | `integer` | نعم | — |
| `channel_limit` | `integer` | نعم | — |
| `internal_ai_cost_budget_yer` | `integer` | نعم | — |
| `status` | `text` | نعم | `'DRAFT'::text` |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `retired_at` | `timestamp with time zone` | لا | — |
| `included_channel_limit` | `integer` | نعم | `1` |
| `channel_ceiling` | `integer` | نعم | `1` |
| `seat_limit` | `integer` | نعم | `1` |

**القيود المثبتة:** `plans_ai_catalog_limit_nonneg_chk` (c)، `plans_ai_reply_limit_positive_chk` (c)، `plans_billing_interval_chk` (c)، `plans_channel_ceiling_chk` (c)، `plans_channel_limit_positive_chk` (c)، `plans_code_not_blank_chk` (c)، `plans_code_version_uq` (u)، `plans_display_name_not_blank_chk` (c)، `plans_included_channel_limit_positive_chk` (c)، `plans_internal_ai_cost_budget_positive_chk` (c)، `plans_pkey` (p)، `plans_price_positive_chk` (c)، `plans_retired_chk` (c)، `plans_seat_limit_positive_chk` (c)، `plans_status_chk` (c). **فهارس UNIQUE مستقلة:** `plans_one_active_version_per_code_uq`.


### `subscription_ai_usage` — billing · ترحيل `000062`

الأعمدة: **12** · القيود: **8** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `subscription_id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `ai_reply_limit` | `integer` | نعم | — |
| `ai_replies_used` | `integer` | نعم | `0` |
| `input_tokens` | `bigint` | نعم | `0` |
| `cached_input_tokens` | `bigint` | نعم | `0` |
| `output_tokens` | `bigint` | نعم | `0` |
| `model_requests` | `integer` | نعم | `0` |
| `tool_calls` | `integer` | نعم | `0` |
| `provider_cost_yer` | `integer` | نعم | `0` |
| `last_recorded_at` | `timestamp with time zone` | لا | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `subscription_ai_usage_business_id_fkey` (f)، `subscription_ai_usage_business_subscription_fk` (f)، `subscription_ai_usage_cost_nonneg_chk` (c)، `subscription_ai_usage_counts_nonneg_chk` (c)، `subscription_ai_usage_pkey` (p)، `subscription_ai_usage_replies_nonneg_chk` (c)، `subscription_ai_usage_subscription_id_fkey` (f)، `subscription_ai_usage_tokens_nonneg_chk` (c).


### `subscriptions` — billing · ترحيل `000062`

الأعمدة: **25** · القيود: **18** · الفهارس الفريدة المستقلة: **2**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `plan_id` | `uuid` | نعم | — |
| `period_start` | `timestamp with time zone` | نعم | — |
| `period_end` | `timestamp with time zone` | نعم | — |
| `status` | `text` | نعم | `'PENDING'::text` |
| `ai_reply_limit` | `integer` | نعم | — |
| `ai_catalog_limit` | `integer` | نعم | — |
| `channel_limit` | `integer` | نعم | — |
| `internal_ai_cost_budget_yer` | `integer` | نعم | — |
| `cost_budget_override_yer` | `integer` | لا | — |
| `cost_budget_override_reason` | `text` | لا | — |
| `cost_budget_override_by` | `uuid` | لا | — |
| `cost_budget_override_at` | `timestamp with time zone` | لا | — |
| `cancelled_at` | `timestamp with time zone` | لا | — |
| `cancelled_reason` | `text` | لا | — |
| `cancelled_by` | `uuid` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |
| `price_yer_snapshot` | `integer` | نعم | — |
| `amount_due_yer` | `integer` | نعم | — |
| `included_channel_limit` | `integer` | نعم | `1` |
| `channel_ceiling` | `integer` | نعم | `1` |
| `seat_limit` | `integer` | نعم | `1` |
| `activated_at` | `timestamp with time zone` | لا | — |

**القيود المثبتة:** `subscriptions_activation_state_chk` (c)، `subscriptions_ai_catalog_limit_nonneg_chk` (c)، `subscriptions_ai_reply_limit_positive_chk` (c)، `subscriptions_amount_due_positive_chk` (c)، `subscriptions_business_id_fkey` (f)، `subscriptions_business_id_id_uq` (u)، `subscriptions_cancelled_chk` (c)، `subscriptions_channel_ceiling_chk` (c)، `subscriptions_channel_limit_positive_chk` (c)، `subscriptions_cost_budget_override_chk` (c)، `subscriptions_included_channel_limit_positive_chk` (c)، `subscriptions_internal_ai_cost_budget_positive_chk` (c)، `subscriptions_period_chk` (c)، `subscriptions_pkey` (p)، `subscriptions_plan_id_fkey` (f)، `subscriptions_price_snapshot_positive_chk` (c)، `subscriptions_seat_limit_positive_chk` (c)، `subscriptions_status_chk` (c). **فهارس UNIQUE مستقلة:** `subscriptions_one_active_per_business_uq`، `subscriptions_one_pending_per_business_uq`.


### `sales_confirmation_requests` — sales · ترحيل `000074`

الأعمدة: **14** · القيود: **10** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `workflow_id` | `uuid` | نعم | — |
| `customer_id` | `uuid` | نعم | — |
| `transaction_id` | `uuid` | نعم | — |
| `transaction_version` | `bigint` | نعم | — |
| `confirm_token_hash` | `text` | نعم | — |
| `decline_token_hash` | `text` | نعم | — |
| `status` | `text` | نعم | — |
| `expires_at` | `timestamp with time zone` | نعم | — |
| `source_message_id` | `uuid` | لا | — |
| `response_message_id` | `uuid` | لا | — |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `sales_confirmation_requests_business_id_customer_id_fkey` (f)، `sales_confirmation_requests_business_id_id_key` (u)، `sales_confirmation_requests_business_id_response_message_i_fkey` (f)، `sales_confirmation_requests_business_id_source_message_id_fkey` (f)، `sales_confirmation_requests_business_id_transaction_id_fkey` (f)، `sales_confirmation_requests_business_id_workflow_id_fkey` (f)، `sales_confirmation_requests_confirm_token_hash_key` (u)، `sales_confirmation_requests_decline_token_hash_key` (u)، `sales_confirmation_requests_pkey` (p)، `sales_confirmation_requests_status_check` (c). **فهارس UNIQUE مستقلة:** `sales_confirmation_pending_idx`.


### `sales_transaction_requirements` — sales · ترحيل `000074`

الأعمدة: **4** · القيود: **5** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `business_id` | `uuid` | نعم | — |
| `transaction_type` | `text` | نعم | — |
| `schema_version` | `integer` | نعم | — |
| `definitions` | `jsonb` | نعم | — |

**القيود المثبتة:** `sales_transaction_requirements_business_id_fkey` (f)، `sales_transaction_requirements_definitions_check` (c)، `sales_transaction_requirements_pkey` (p)، `sales_transaction_requirements_schema_version_check` (c)، `sales_transaction_requirements_transaction_type_check` (c).


### `sales_workflows` — sales · ترحيل `000074`

الأعمدة: **14** · القيود: **10** · الفهارس الفريدة المستقلة: **0**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `customer_id` | `uuid` | نعم | — |
| `conversation_id` | `uuid` | لا | — |
| `lead_id` | `uuid` | لا | — |
| `source_message_id` | `uuid` | لا | — |
| `pending_action` | `text` | لا | — |
| `transaction_id` | `uuid` | لا | — |
| `state` | `text` | نعم | — |
| `proposal` | `jsonb` | نعم | `'{}'::jsonb` |
| `policy_version` | `bigint` | نعم | — |
| `resource_version` | `bigint` | نعم | `1` |
| `created_at` | `timestamp with time zone` | نعم | — |
| `updated_at` | `timestamp with time zone` | نعم | — |

**القيود المثبتة:** `sales_workflows_business_id_conversation_id_fkey` (f)، `sales_workflows_business_id_customer_id_fkey` (f)، `sales_workflows_business_id_id_key` (u)، `sales_workflows_business_id_lead_id_fkey` (f)، `sales_workflows_business_id_source_message_id_fkey` (f)، `sales_workflows_business_id_transaction_id_fkey` (f)، `sales_workflows_pkey` (p)، `sales_workflows_proposal_check` (c)، `sales_workflows_resource_version_check` (c)، `sales_workflows_state_check` (c).


### `subscription_addons` — billing · ترحيل `000077`

الأعمدة: **18** · القيود: **13** · الفهارس الفريدة المستقلة: **1**.

| العمود | نوع PostgreSQL النهائي | NOT NULL | Default |
|---|---|---|---|
| `id` | `uuid` | نعم | — |
| `subscription_id` | `uuid` | نعم | — |
| `business_id` | `uuid` | نعم | — |
| `kind` | `text` | نعم | — |
| `price_yer_snapshot` | `integer` | نعم | — |
| `ai_reply_delta` | `integer` | نعم | `0` |
| `catalog_delta` | `integer` | نعم | `0` |
| `seat_delta` | `integer` | نعم | `0` |
| `channel_delta` | `integer` | نعم | `0` |
| `status` | `text` | نعم | — |
| `activated_at` | `timestamp with time zone` | لا | — |
| `cancelled_at` | `timestamp with time zone` | لا | — |
| `cancelled_reason` | `text` | لا | — |
| `created_by` | `uuid` | نعم | — |
| `created_at` | `timestamp with time zone` | نعم | `now()` |
| `updated_at` | `timestamp with time zone` | نعم | `now()` |
| `creation_idempotency_key` | `text` | لا | — |
| `cancelled_by` | `uuid` | لا | — |

**القيود المثبتة:** `subscription_addons_activation_state_chk` (c)، `subscription_addons_ai_reply_delta_check` (c)، `subscription_addons_business_subscription_fk` (f)، `subscription_addons_cancelled_state_chk` (c)، `subscription_addons_catalog_delta_check` (c)، `subscription_addons_channel_delta_check` (c)، `subscription_addons_kind_check` (c)، `subscription_addons_kind_delta_chk` (c)، `subscription_addons_pkey` (p)، `subscription_addons_price_yer_snapshot_check` (c)، `subscription_addons_seat_delta_check` (c)، `subscription_addons_status_check` (c)، `subscription_addons_subscription_id_fkey` (f). **فهارس UNIQUE مستقلة:** `subscription_addons_creation_idempotency_idx`.

## 4. أعمدة تتطلب مراجعة حساسية يدوية صريحة

الكشف التالي مبني على **أسماء الحقول فقط**، وليس دليلًا على أن كل حقل غير مذكور آمن. لا يسمح بأي حقل من المصادر المرشحة حتى تُعتمد allowlist دقيقة بعد تصنيف PII والسرية ومصدر المعلومة وحاجة المهمة.

- `channel_connections.provider_ref`
- `channel_connections.provider_account_ref`
- `channel_connections.provider_connection_ref`
- `channel_connections.secret_reference`
- `customers.profile`
- `customers.contact_points`
- `communication_messages.provider_message_id`
- `automation_rules.action_payload`
- `subscription_ai_usage.input_tokens`
- `subscription_ai_usage.cached_input_tokens`
- `subscription_ai_usage.output_tokens`
- `subscription_ai_usage.provider_cost_yer`
- `sales_confirmation_requests.confirm_token_hash`
- `sales_confirmation_requests.decline_token_hash`

## 5. الممنوعات والمطابقة

- الجداول الثلاثة التي حذفها ترحيل `000053` ليست في snapshot النهائي: `chatwoot_workspace_bindings`، `chatwoot_contact_links`، `chatwoot_mirror_jobs`.
- العمود `attribute_definitions.is_searchable` غير موجود بعد ترحيل `000068`.
- تتضمن صورة PostgreSQL أيضًا الجداول الداخلية والحساسة؛ **الحفظ في ملف اختبارات لا يعني إرسالها إلى LLM**.
- لا يستخدم AI حقول `resource_version` أو القيم `NULL` دون احترام شروط التزامن والاكتفاء وفترة الصلاحية حسب عقد المجال.
- الجداول المصنفة `internal_only` أو `prohibited` أو `retired` تبقى مستبعدة من الإسقاطات تمامًا.

## 6. بوابة الانتقال إلى عقود المجالات

1. اعتماد قائمة الحقول المسموحة لكل إسقاط، والغرض منه، وقواعد redaction/PII.
2. مطابقة القيم المغلقة وCHECK/FK الفعلية مع كود Go وports وخدمات كل مجال.
3. تنفيذ قواعد القراءة المقيدة بالمنشأة والصلاحيات، مع مراجع موارد مصدرها الباك إند.
4. اختبار منع الحقول غير المدرجة، ورفض السجلات من منشآت أخرى.
5. لا تشغيل أدوات قراءة AI ولا توسيع `Registry` حتى اجتياز ما سبق.

**تاريخ هذه الوثيقة:** 2026-10-25. **الحالة:** مراجعة بنية PostgreSQL مكتملة في قاعدة الاختبار؛ مراجعة الحقول الدلالية والأمان لكل مجال لم تُغلق بعد.
