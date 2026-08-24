# Domain Contract — Channel وIdentity وCommunication

## قرار الملكية

هذا الجزء يحدد ما يحتاجه مجيب 24 ليشغّل القنوات، لكنه لا يعيد بناء SocialAPI أو Chatwoot.

```text
SocialAPI/Provider IDs     = External References
Chatwoot Conversation      = Workspace Reference
Mujeeb Conversation        = Sales/Mapping Reference
Mujeeb Message record     = Reliability/processing reference
```

Chatwoot يملك Communication Workspace Operations مثل Inbox وTeam وAssignment وLabel وConversation UI. مجيب 24 يحتفظ بالمراجع والحالة التي يحتاجها للمبيعات والتزامن، لا بنموذج Chatwoot كامل.

## 1. channel/

### 1.1 ChannelConnection Aggregate

يمثل اتصال Business واحدًا بحساب خارجي عبر Provider.

```text
ChannelConnection
├── id
├── business_id
├── provider
├── channel
├── provider_account_id
├── provider_connection_id
├── status
├── capabilities
├── secret_reference
├── last_health_check_at
├── last_provider_event_at
├── created_at
└── updated_at
```

القيم:

```text
provider: socialapi | meta_direct | future_provider
channel: facebook | instagram | whatsapp
status: pending | connecting | active | degraded |
        disconnected | reconnect_required | failed
```

`provider_account_id` هو الحساب/الصفحة عند المزود، و`provider_connection_id` هو اتصال Provider. كلاهما opaque string؛ لا نعتمد على شكله أو نشتق منه Business.

### 1.2 ChannelConnection Invariants

لا يمكن إنشاء Connection بلا Business أو Provider أو Channel أو Provider Connection ID. يجب أن يكون الزوج:

```text
(provider, provider_connection_id)
```

فريدًا في النظام، حتى لا يرتبط حساب خارجي بتاجرين بطريقة غير مقصودة.

لا تخزن `secret_reference` السر نفسه؛ هي إشارة إلى SecretStore. لا يظهر token في Domain logs أو events أو API responses.

لا تصبح Connection `active` إلا بعد نجاح اختبار الاتصال وتأكيد القدرة الأساسية المطلوبة للقناة. لا يعني `active` أن كل capabilities متاحة؛ لكل قدرة حالة مستقلة.

لا نحذف Connection عند disconnect. نغير status إلى `disconnected` أو `reconnect_required` ونحتفظ بالمراجع والأحداث التاريخية.

### 1.3 ChannelConnection Lifecycle

```text
pending → connecting → active
active → degraded
active → disconnected
active → failed
 degraded → active
 degraded → disconnected
 disconnected → reconnect_required
 reconnect_required → connecting
 failed → connecting
```

لا نسمح بالانتقال إلى `active` مباشرة من `failed` دون محاولة اتصال ناجحة. ولا نسمح بإرسال outbound من Connection حالتها `disconnected` أو `failed`.

`degraded` لا تعني إيقاف كل شيء بالضرورة؛ قد تستمر القراءة بينما يتوقف الإرسال إذا كانت capability المتأثرة هي `send_messages`.

### 1.4 Capabilities

```text
receive_messages
send_messages
receive_comments
reply_comments
private_reply
media_inbound
media_outbound
interactive_messages
templates
delivery_status
```

نستخدم Enum/typed value وليس JSON عشوائيًا في قواعد القرار. يعتمد V1 على أسماء دقيقة قابلة للقراءة والتدقيق، ومنها `media_inbound` و`media_outbound` بدل capability تجميعية باسم `media`، حتى لا يفقد القرار اتجاه النقل. يمكن تخزين metadata إضافية في Adapter، لكن Application يستعمل capability contract موحدًا.

كل Capability تحمل:

```text
name
enabled
checked_at
source
```

`enabled=false` يعني Unsupported أو غير مكتشفة، وليس خطأ مؤقتًا تلقائيًا. حالة الصحة المؤقتة تذهب إلى Health/Integration state.

### 1.5 Channel Domain Events

```text
ChannelConnectionCreated
ChannelConnectionActivated
ChannelConnectionDegraded
ChannelConnectionDisconnected
ChannelReconnectRequired
ChannelCapabilityChanged
ChannelHealthCheckFailed
```

لا يحتوي الحدث على access token أو raw provider payload.

## 2. InboundEvent

### 2.1 InboundEvent كـDurable Domain Record

الحدث الوارد هو سجل قبول ومعالجة، وليس Conversation ولا رسالة Chatwoot.

```text
InboundEvent
├── id
├── business_id
├── connection_id
├── provider
├── provider_event_id
├── event_type
├── interaction_kind
├── provider_message_id
├── provider_conversation_id
├── external_user_id
├── content_reference
├── raw_payload_reference
├── signature_verified
├── occurred_at
├── received_at
└── processing_state
```

`interaction_kind`:

```text
dm | comment | story_reply | mention | review | postback | other
```

`interaction_kind=comment` لا ينشئ Conversation تلقائيًا. Conversation Resolution Policy هي التي تقرر: existing، create، أو no conversation.

### 2.2 InboundEvent Invariants

المفتاح المنطقي للتكرار:

```text
provider + provider_connection_id + provider_event_id
```

إذا لم يرسل Provider Event ID موثوقًا، لا نخمنه من النص وحده؛ نستخدم canonical fingerprint موثقًا ونضع event في حالة تحتاج reconciliation إذا كان احتمال التصادم مرتفعًا.

لا يعالج InboundEvent AI أو يرسل ردًا بنفسه. هو يثبت القبول ويشير إلى خطوات لاحقة.

`signature_verified` يجب أن يكون true قبل أن يدخل الحدث إلى pipeline الطبيعي. أحداث verification/handshake لها مسار منفصل.

### 2.3 InboundEvent Lifecycle

```text
received → processing → processed
received → rejected
processing → failed
failed → retrying → processing
failed → dead_letter
```

`processed` يعني اكتمال الخطوات الداخلية المطلوبة، وليس أن AI أجاب أو أن العميل استلم ردًا.

### 2.4 Inbound Domain Events

```text
InboundEventAccepted
InboundEventDuplicated
InboundEventRejected
InboundEventProcessingFailed
InboundEventProcessed
```

نحتفظ بـ`raw_payload_reference` خارج Domain object الكامل. يمكن إعادة التطبيع عند تغير Adapter أو اكتشاف field جديد.

## 3. identity/

### 3.1 Customer Aggregate

Customer هو العميل داخل مساحة Business في مجيب 24.

```text
Customer
├── id
├── business_id
├── display_name
├── contact_points
├── commercial_profile_reference
├── status
├── created_at
└── updated_at
```

لا يحتوي Customer على كل provider payload. `commercial_profile_reference` يشير إلى Sales/Customer Context لاحقًا.

الحالات:

```text
active | blocked | merged | archived
```

### 3.2 ExternalIdentity

يمثل هوية العميل في قناة خارجية:

```text
ExternalIdentity
├── id
├── business_id
├── customer_id
├── connection_id
├── provider
├── provider_account_id
├── channel
├── external_user_id
├── display_name
├── verified_contact_reference
├── confidence
├── status
├── created_at
└── updated_at
```

المفتاح الفريد:

```text
provider + provider_account_id + channel + external_user_id
```

لا ندمج Instagram identity وWhatsApp identity من الاسم فقط. الدمج يحتاج phone/email verified أو قاعدة ثقة صريحة وتسجيل سبب الدمج.

`confidence` لا يحول تخمين AI إلى حقيقة. الدمج منخفض الثقة يظل منفصلًا أو يحتاج human review.

### 3.3 Identity Lifecycle

```text
observed → linked
observed → needs_review
linked → merged
linked → blocked
linked → archived
```

لا نحذف ExternalIdentity التاريخية عند تغير اسم المستخدم أو disconnect. نحدث status والمراجع.

### 3.4 Identity Domain Events

```text
CustomerCreated
ExternalIdentityObserved
ExternalIdentityLinked
ExternalIdentityMergeRequested
ExternalIdentityMerged
CustomerBlocked
CustomerArchived
```

## 4. communication/

### 4.1 ConversationReference

هذه ليست نسخة من Chatwoot Conversation. هي مرجع موحد يربط المحادثة التجارية بمراجع Workspace وProvider.

```text
ConversationReference
├── id
├── business_id
├── customer_id
├── connection_id
├── mujeeb_conversation_id
├── provider_conversation_id
├── provider_thread_key
├── chatwoot_account_id
├── chatwoot_inbox_id
├── chatwoot_contact_id
├── chatwoot_conversation_id
├── channel
├── conversation_kind
├── mapping_status
├── ownership_reference
├── created_at
└── updated_at
```

`conversation_kind`:

```text
dm | comment_thread | story_reply | mention | review | other
```

`mapping_status`:

```text
pending | mapped | degraded | rebuild_required | closed
```

`mapping_status=closed` يعكس انتهاء المرجع التشغيلي في Mujeeb، ولا يفرض على Chatwoot status معينًا إلا عبر Adapter/Application command.

### 4.2 Conversation Invariants

لا يوجد أكثر من Reference فعال لنفس:

```text
business + connection + provider_conversation_id + conversation_kind
```

إذا لم يملك Provider Conversation ID، نستخدم `provider_thread_key` أو سياسة قناة موثقة. لا ننشئ Conversation جديدة لمجرد فشل حفظ mapping؛ نبحث أولًا بالمفاتيح الخارجية وChatwoot references.

ConversationReference لا يملك Team أو Label أو Inbox ككيانات Domain. يحتفظ فقط بـIDs ومراجع.

### 4.3 Ownership Reference

إذا احتاج Sales Domain معرفة المالك، نستخدم:

```text
OwnershipReference
├── agent_reference
├── team_reference
├── source
└── updated_at
```

لا ننشئ `Team` أو `ChatwootAgent` داخل Domain.

### 4.4 CommunicationMessageReference

لا نبني Message Domain كاملًا ينافس Chatwoot، لكن نحتاج سجلًا داخليًا للموثوقية والربط:

```text
CommunicationMessageReference
├── id
├── business_id
├── conversation_reference_id
├── inbound_event_id (optional)
├── outbound_message_id (optional)
├── direction
├── origin
├── transport
├── provider_message_id (optional)
├── chatwoot_message_id (optional)
├── content_type
├── text_content (optional; text-only V1)
├── content_reference
├── occurred_at
└── created_at
```

هذا المرجع لا يقرر UI أو Team أو Label. `outbound_message_id` يربطه اختياريًا بسجل نية الإرسال، و`inbound_event_id` يربطه اختياريًا بحدث الاستقبال؛ لا يعني ذلك دمج lifecycle أو نسخ Provider/Chatwoot. `content_reference` يربط محتوى أو مرفقات خارج السجل، و`text_content` يغطي text-only V1. لا يستخدم السجل `map[string]any` كبديل عن Contract، ولا يتبنى DTO Chatwoot.

القيم:

```text
direction: inbound | outbound
origin: customer | ai | human | automation | system
transport: provider | chatwoot | mujeeb
```

هذه الثلاثة تمنع Loop: إذا جاء Chatwoot Webhook لرسالة outbound أنشأها Go، نربطها بالمرجع الموجود ولا نرسلها إلى Provider مرة ثانية.

`CommunicationMessageReference` لا يملك عمود `status` مستقلًا في V1 حتى لا يكرر OutboundMessage delivery lifecycle. عند إسقاط timeline للقراءة، تكون الرسالة الواردة غير المرتبطة بـOutboundMessage ذات status عرضي `received`، وتكون الرسالة غير المرتبطة بـOutboundMessage ذات status عرضي `recorded`؛ أما الرسالة المرتبطة بـ`outbound_message_id` فتستخدم حالة OutboundMessage الحالية (`pending | sending | accepted | sent | delivered | read | failed | unknown`). `received` و`recorded` ليسا حالات OutboundMessage ولا يدّعيان وصول الرسالة إلى العميل.

### 4.5 Communication Domain Events

```text
ConversationReferenceCreated
ConversationMapped
ConversationMappingRepairRequired
CommunicationMessageRecorded
OwnershipReferenceChanged
```

## 5. Dependencies بين الأجزاء

```text
Business
  → ChannelConnection
      → InboundEvent
          → ExternalIdentity / Customer
              → ConversationReference
                  → CommunicationMessageReference
```

لكن هذه علاقات IDs وReferences وليست Aggregate عملاقًا. لا نحمّل Business بكل المحادثات والعملاء في الذاكرة.

## 6. معايير الإغلاق

نعتبر هذا العقد مغلقًا عندما نوافق على:

1. Enum القنوات والحالات والقدرات.
2. مفتاح Idempotency للحدث الوارد.
3. قاعدة عدم حذف Connection وExternalIdentity التاريخية.
4. الفرق بين Interaction Kind وConversation Kind.
5. ConversationReference كمرجع لا كنسخة Chatwoot.
6. CommunicationMessageReference كسجل ربط وموثوقية، لا Communication Workspace جديد.
7. OwnershipReference بدل Team/Assignment domain كامل.
8. منع دمج هويات القنوات بالاسم فقط.
9. حالات `unknown/rebuild_required` للحالات الغامضة.
10. Domain Events بلا أسرار أو raw provider payload.

بعد تثبيت هذا العقد وتنفيذ migration وMessageRepository وListConversationMessages integration tests، ننتقل إلى `domain/catalog` و`domain/sales`، مع بقاء هذه الطبقات مستقلة عن Provider وChatwoot. Migration `000028` هي تصحيح forward-only ولا تعدّل `000001–000027`؛ وقد ثبتت على PostgreSQL 16 مع tenant FKs وkeyset pagination وRecord/List mapping.
