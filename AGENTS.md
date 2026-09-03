# تعليمات AI/Agent لمشروع Mujeeb 24

قبل أي قراءة تحليلية أو تعديل أو تشغيل، يجب قراءة هذا الملف ثم الوثيقة المرجعية الكاملة:

`docs/architecture/chatwoot-free-ai-handoff-prompt-ar.md`

القرار الأساسي غير قابل للتفاوض: **Chatwoot متروك نهائيًا**. مصدر الحقيقة هو Mujeeb/PostgreSQL، وSocialAPI ناقل خارجي فقط. لا تعِد Chatwoot، ولا تضف payment أو shipping أو Dashboard في هذه المرحلة، ولا تمنح Platform Super Admin وصولًا صامتًا عبر التجار.

بعد قراءة المرجع، اقرأ `README` و`todo.md` ووثائق architecture ذات الصلة، وافحص `git status` والفرع والـdiff. تعامل مع الكود والاختبارات كمصدر تحقق من الحالة الحالية، ولا تعتبر أي وثيقة دليلًا بديلًا عن التنفيذ.

القواعد الإلزامية:

- لا أسرار أو كلمات مرور أو raw invite tokens في الملفات أو logs أو المحادثة.
- لا network calls داخل PostgreSQL transaction، ولا blind retry عند نتيجة خارجية غير معروفة.
- احفظ tenant/business scope وidempotency في كل مسار.
- طبّق Single Responsibility وافصل HTTP وapplication وpersistence وprovider.
- استخدم forward-only migrations فقط، ولا تعدّل migrations التاريخية أو تحذف بيانات بلا موافقة.
- ميّز بدقة بين unit test وPostgreSQL integration وprovider contract وlive smoke وproduction readiness.
- إذا لم يوجد `POSTGRES_TEST_DSN` فاختبار PostgreSQL **skipped** وليس passed.
- لا تنشئ commit أو push أو merge إلى `main` بلا موافقة صريحة.

إذا تعارض طلب جديد مع هذه القواعد، أوقف التنفيذ واشرح التعارض والملفات والاختبارات المطلوبة قبل أي تغيير.

> هذا الملف والوثيقة المرجعية لا يقدمان ضمانًا مطلقًا من الأخطاء؛ الدليل هو الكود والاختبارات ونتائج التشغيل الموثقة.
