# OlymPI Roadmap

File này ghi lại những gì OlymPI đã có, đang làm và sẽ làm. Thứ tự phản ánh ưu tiên hiện tại và có thể đổi theo phản hồi từ người dùng.

## Đã có

- **Operator panel:** điều khiển round, câu hỏi, timer, bell lock, Đúng/Sai, ghi điểm và undo.
- **Audience display:** các scene theo từng round, bell, điểm và timer, âm thanh chạy qua trình duyệt.
- **Contestant station:** đăng nhập bằng slot và PIN 6 số, bấm bell và trả lời.
- **Question sets có version:** mỗi lần save tạo một revision mới; trận đã chọn giữ revision đang dùng.
- **Match lifecycle rõ ràng:** ready, playing, paused, completed.
- **Event log và replay** cho từng trận.
- **Một binary duy nhất** có sẵn web UI bên trong; build Linux và Windows qua GitHub Actions.

## Đang làm: rule pack

Luật chơi thay đổi theo từng mùa, và không chỉ là số điểm. Có khi cả cấu trúc round cũng đổi. Cách tiếp cận:

- **Round types nằm trong code.** Ví dụ: personal question round, common buzzer round, keyword round có reveal rows, timed-rank round, finish round có packages, knockout tiebreaker. Mỗi loại có screen và controls riêng.
- **Rule pack là một file dữ liệu.** Pack liệt kê các round theo thứ tự và đặt parameters cho từng round: số câu, số contestant, time limit, quyền bấm bell, scoring formula, penalty và floor.
- **Thay đổi lớn cần release mới.** Nếu cần một cơ chế mới cho round, phải thêm vào code trước rồi pack mới mới dùng được.
- **Mỗi match lưu rule pack lúc tạo.** Đổi luật sau đó không ảnh hưởng match đang chạy hay replay.

Milestone đầu tiên là mô hình hóa season 24 của Đường lên đỉnh Olympia thành một rule pack, kèm golden test. Test chạy một match mẫu có điểm đã biết (ví dụ tổng tối đa 800 điểm) và phải pass sau mỗi thay đổi.

Milestone tiếp theo là steal chain ở round Về đích. Sau khi contestant trả lời sai hoặc hết giờ, các contestant còn lại lần lượt có cơ hội bấm bell. Chuỗi này tiếp tục cho đến khi có người trả lời đúng hoặc hết contestant.

## Sắp tới

- **Theme pack.** Màu sắc, background, logo, font, âm thanh và tên sự kiện đọc từ một file, không cần build lại. Hiện tại các giá trị này nằm trong `brand.ts` và `brand.go`.
- **Hướng dẫn theo vai trò.** Đã có hướng dẫn setup; cần thêm guide cho contestant và operator.
- **LAN dễ setup hơn.** Hiện tại dùng HTTP trên LAN cần flag `--allow-insecure-lan`, còn HTTPS cần self-signed certificate. Cần một cách setup đơn giản hơn cho event.
- **Test trên thiết bị thật.** Nhiều browser và thiết bị (tablet, phone, TV) trên cùng một mạng có nhiều máy.

## Về sau

- **Rule pack editor.** Giao diện tạo và sửa rule pack ngay trong app. Chỉ làm khi schema đã ổn định, để tránh làm lại nhiều lần.
- **Public pack registry.** Nơi chia sẻ rule pack và theme pack, kèm hash để kiểm tra. Bước đầu chỉ là import và export file.
- **Âm thanh và font thay thế.** Bản public dùng sound câm và font mở; người dùng có thể thay bằng tài nguyên của mình.

## Không làm

- Không chạy code tùy ý từ rule pack hay theme pack. Pack chỉ là data, và được validate trước khi dùng.
- Không làm hosted service tập trung cho từng ban tổ chức ở giai đoạn này.

## Góp ý

Mở issue trên GitHub nếu gặp lỗi hoặc muốn góp ý về luật, giao diện. Với luật, nên ghi rõ season, round và nguồn đối chiếu nếu có.
