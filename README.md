# OlymPI

Máy chủ thi đấu trực tiếp theo thể thức Olympia. Một tệp thực thi Go duy nhất phục vụ ba màn hình web:

- **Bảng điều khiển BTC:** điều khiển vòng thi, câu hỏi, đồng hồ, khóa chuông, chấm đúng/sai và điểm số.
- **Màn hình khán giả:** màn hình TV với các cảnh cho từng vòng thi.
- **Trạm thí sinh:** màn hình riêng cho từng thí sinh để trả lời và bấm chuông.

Giao diện web được viết bằng SvelteKit và nhúng sẵn vào tệp Go, nên không cần triển khai thành phần nào khác. Dữ liệu được lưu trong SQLite tại thư mục `4t_data/`.

## Khởi chạy nhanh

Yêu cầu: Go (phiên bản ghi trong `go.mod`), Node.js 22 trở lên, pnpm 11 và [go-task](https://taskfile.dev).

```sh
cd ui && pnpm install && cd ..
task build
./dist/olympi
```

Lần chạy đầu tiên sẽ mở trang thiết lập và tạo các tài khoản thí sinh `thi_sinh_1` đến `thi_sinh_4`. Mã PIN 6 số dùng một lần của từng tài khoản được in ra terminal. Hãy lưu lại và đổi trong mục Cài đặt.

Các lệnh thường dùng:

| Lệnh | Tác dụng |
| --- | --- |
| `task dev` | Chạy máy chủ Go và giao diện Vite cùng lúc |
| `task build` | Build giao diện và tệp `dist/olympi` |
| `task test` | Test Go, kiểm tra kiểu TypeScript, lint và test giao diện |
| `task build-linux` / `task build-windows` | Build tệp chạy cho Linux và Windows (amd64) |

## Đổi thương hiệu

Bản công khai dùng logo, nền và âm thanh tạm thời. Để đổi sang thương hiệu của bạn:

1. Sửa `ui/src/lib/brand.ts` (tên hiển thị trên màn hình) và `internal/settings/brand.go` (tên sự kiện mặc định).
2. Thay các tệp logo và nền trong `ui/static/` (`4t.png`, `4t-blue.png`, `logo-school.png`, `bg-*`, `rule.png`, `star.gif`) và trong `ui/src/lib/assets/image/`. Giữ nguyên tên tệp để không phải sửa mã.
3. Thay âm thanh trong `ui/static/sounds/`, giữ nguyên tên và định dạng tệp. Các âm thanh đi kèm là tệp câm một giây, dùng làm chỗ giữ.
4. Thay phông chữ hiển thị `ui/static/fonts/display-*.ttf`. Hiện tại là Montserrat, theo giấy phép SIL Open Font License.
5. Chạy lại `task build`.

## Giấy phép

Mã nguồn theo [PolyForm Noncommercial 1.0.0](LICENSE). Bạn được sử dụng, sao chép và chỉnh sửa cho mục đích phi thương mại, ví dụ trong câu lạc bộ, trường học hoặc sự kiện cá nhân. Sử dụng thương mại cần thỏa thuận riêng với tác giả.

Các thành phần bên thứ ba giữ giấy phép riêng (xem trong từng gói). Phông Selawik và Montserrat theo SIL Open Font License.
